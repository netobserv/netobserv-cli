#!/usr/bin/env bash

set -e
set +u

# e2e skips inputs
if [ -z "${isE2E+x}" ]; then isE2E=false; fi
# keep capture state
if [ -z "${captureStarted+x}" ]; then captureStarted=false; fi
# prompt copy by default
if [ -z "${copy+x}" ]; then copy="prompt"; fi
# run foreground by default
if [ -z "${runBackground+x}" ]; then runBackground="false"; fi
# output as yaml only
if [ -z "${outputYAML+x}" ]; then outputYAML="false"; fi
# formated date for file names
if [ -z "${dateName+x}" ]; then dateName="$(date +"%Y_%m_%d_%I_%M")"; fi

# force skipping cleanup
skipCleanup=false

# TLS enabled on OpenShift
tlsEnabled=false

# get either oc (favorite) or kubectl paths
# this is used only when calling commands directly
# else it will be overridden by inject.sh
K8S_CLI_BIN_PATH=$(which oc 2>/dev/null || which kubectl 2>/dev/null)
K8S_CLI_BIN=$(basename "${K8S_CLI_BIN_PATH}")

# namespace for this run
namespace="netobserv-cli"

if [ -n "$NETOBSERV_NAMESPACE" ]; then
  echo "using custom namespace $NETOBSERV_NAMESPACE"
  namespace="$NETOBSERV_NAMESPACE"
fi

# collector target host
targetHost="collector.$namespace.svc.cluster.local"

# eBPF agent image to use
agentImg="quay.io/netobserv/netobserv-ebpf-agent:main"

if [ -n "$NETOBSERV_AGENT_IMAGE" ]; then
  echo "using custom agent image $NETOBSERV_AGENT_IMAGE"
  agentImg="$NETOBSERV_AGENT_IMAGE"
fi

# globals populated from calling script
command=""
options=""
manifest=""
nodeSelector=""

OUTPUT_PATH="./output"
YAML_OUTPUT_FILE="capture.yml"
MANIFEST_OUTPUT_PATH=$(mktemp -d)
FLOWS_MANIFEST_FILE="flow-capture.yml"
PACKETS_MANIFEST_FILE="packet-capture.yml"
METRICS_MANIFEST_FILE="metric-capture.yml"
CONFIG_JSON_TEMP="config.json"
CLUSTER_CONFIG="cluster-config-v1.yaml"
NETWORK_CONFIG="cluster-network.yaml"

function loadYAMLs() {
  namespaceYAML='
    namespaceYAMLContent
  '
  if [ -f ./res/namespace.yml ]; then
    namespaceYAML="$(cat ./res/namespace.yml)"
  fi
  namespaceYAML="${namespaceYAML/"{{NAME}}"/${namespace}}"

  saYAML='
    saYAMLContent
  '
  if [ -f ./res/service-account.yml ]; then
    saYAML="$(cat ./res/service-account.yml)"
  fi
  saYAML="${saYAML//"{{NAMESPACE}}"/${namespace}}"

  flowAgentYAML='
    flowAgentYAMLContent
  '
  if [ -f ./res/flow-capture.yml ]; then
    flowAgentYAML="$(cat ./res/flow-capture.yml)"
  fi
  flowAgentYAML="${flowAgentYAML/"{{NAMESPACE}}"/${namespace}}"
  flowAgentYAML="${flowAgentYAML/"{{AGENT_IMAGE_URL}}"/${agentImg}}"

  packetAgentYAML='
    packetAgentYAMLContent
  '
  if [ -f ./res/packet-capture.yml ]; then
    packetAgentYAML="$(cat ./res/packet-capture.yml)"
  fi
  packetAgentYAML="${packetAgentYAML/"{{NAMESPACE}}"/${namespace}}"
  packetAgentYAML="${packetAgentYAML/"{{AGENT_IMAGE_URL}}"/${agentImg}}"

  metricAgentYAML='
    metricAgentYAMLContent
  '
  if [ -f ./res/metric-capture.yml ]; then
    metricAgentYAML="$(cat ./res/metric-capture.yml)"
  fi
  metricAgentYAML="${metricAgentYAML/"{{NAMESPACE}}"/${namespace}}"
  metricAgentYAML="${metricAgentYAML/"{{AGENT_IMAGE_URL}}"/${agentImg}}"

  collectorServiceYAML='
    collectorServiceYAMLContent
  '
  if [ -f ./res/collector-service.yml ]; then
    collectorServiceYAML="$(cat ./res/collector-service.yml)"
  fi
  collectorServiceYAML="${collectorServiceYAML/"{{NAMESPACE}}"/${namespace}}"

  smYAML='
    smYAMLContent
  '
  if [ -f ./res/service-monitor.yml ]; then
    smYAML="$(cat ./res/service-monitor.yml)"
  fi
  smYAML="${smYAML//"{{NAMESPACE}}"/${namespace}}"
}

# set pipeline for flows & packets using collector
function setCollectorPipelineConfig() {
  # load pipeline json
  collectorPipelineConfigJSON='
    collectorPipelineConfigJSONContent
  '
  if [ -f ./res/collector-pipeline-config.json ]; then
    collectorPipelineConfigJSON="$(tr <./res/collector-pipeline-config.json '\n' ' ')"
  fi

  # replace target host
  collectorPipelineConfigJSON="${collectorPipelineConfigJSON/"{{TARGET_HOST}}"/${targetHost}}"

  # append json to yaml file
  "$YQ_BIN" e --inplace " .spec.template.spec.containers[0].env[] |= select(.name == \"FLP_CONFIG\").value |= ($collectorPipelineConfigJSON | tojson)" "$1"
}

# set pipeline for metrics
function setMetricsPipelineConfig() {
  # load pipeline json
  metricsPipelineConfigJSON='
    metricsPipelineConfigJSONContent
  '
  if [ -f ./res/metrics-pipeline-config.json ]; then
    metricsPipelineConfigJSON="$(tr <./res/metrics-pipeline-config.json '\n' ' ')"
  fi

  # append json to yaml file
  "$YQ_BIN" e --inplace " .spec.template.spec.containers[0].env[] |= select(.name == \"FLP_CONFIG\").value |= ($metricsPipelineConfigJSON | tojson)" "$1"
}

function enableCollectorTLS() {
  # Add CA volume and mount to DaemonSet for FLP client TLS
  "$YQ_BIN" e --inplace '.spec.template.spec.containers[0].volumeMounts += [{"name":"collector-ca","mountPath":"/etc/collector-ca","readOnly":true}]' "$manifest"
  "$YQ_BIN" e --inplace '.spec.template.spec.volumes += [{"name":"collector-ca","configMap":{"name":"collector-ca"}}]' "$manifest"

  # Add TLS config to FLP pipeline grpc write
  copyFLPConfig "$manifest"
  sendIndex=$("$YQ_BIN" e -oj ".parameters[] | select(.name==\"send\") | path | .[-1]" "$json")
  "$YQ_BIN" e -oj --inplace ".parameters[$sendIndex].write.grpc.tls = {\"caCertPath\":\"/etc/collector-ca/service-ca.crt\"}" "$json"
  updateFLPConfig "$json" "$manifest"

  # Annotate collector service for cert generation
  collectorServiceYAML=$(echo "$collectorServiceYAML" | "$YQ_BIN" e '.metadata.annotations."service.beta.openshift.io/serving-cert-secret-name" = "collector-tls"' -)
}

function createCAConfigMap() {
  applyYAML "apiVersion: v1
kind: ConfigMap
metadata:
  name: collector-ca
  namespace: $namespace
  annotations:
    service.beta.openshift.io/inject-cabundle: \"true\""
}

function clusterIsReady() {
  ready=$(${K8S_CLI_BIN} get all 2>&1 | grep -c "Unable to connect")
  if [[ "${ready}" -gt 0 ]]; then
    return 1
  else
    return 0
  fi
}

function checkClusterVersion() {
  # isOCP is a global consumed later (e.g. to enable collector TLS): default to false and flip it to
  # true once we confirm we're talking to an OpenShift cluster.
  isOCP=false
  states=$(${K8S_CLI_BIN} get clusterversion version -o jsonpath='{.status.history[*].state}')
  if [[ -z "${states}" ]]; then
    echo "Can't check version since cluster is not OpenShift"
  else
    isOCP=true
    versions=$(${K8S_CLI_BIN} get clusterversion version -o jsonpath='{.status.history[*].version}')
    version=""

    # get the current version finding *Completed* state
    if [[ "$(declare -p states)" =~ "declare -a" ]]; then
      # handle states and versions as arrays
      if [ "${#states[@]}" -eq "${#versions[@]}" ]; then
        for i in "${!states[@]}"; do
          if [[ "${states[$i]}" = "Completed" ]]; then
              version="${versions[$i]}"
          fi
        done
      fi
    else
      # handle states and versions as strings
      if [ "${states}" = "Completed" ]; then
          version="${versions}"
      fi
    fi

    if [ -z "${version}" ]; then
      # allow running if no version found since the user may be running an upgrade
      echo "Warning: can't find current version in the clusterversion history"
      echo "Is the cluster upgrading?"
      return 0
    else 
      echo "OpenShift version: $version"
    fi

    returnCode=0
    result=""

    if [[ "$command" = "packets" ]]; then
      compare_versions "$version" 4.16.0
      if [ "$result" -eq 0 ]; then
          echo "- Packet capture requires OpenShift 4.16 or higher"
          returnCode=1
      fi
    fi

    if [[ "${options[*]}" == *"enable_all"* || "${options[*]}" == *"enable_network_events"* ]]; then
      compare_versions "$version" 4.19.0
      if [ "$result" -eq 0 ]; then
          echo "- Network events requires OpenShift 4.19 or higher"
          returnCode=1
      fi
    fi

    if [[ "${options[*]}" == *"enable_all"* || "${options[*]}" == *"enable_udn_mapping"* ]]; then
      compare_versions "$version" 4.18.0
      if [ "$result" -eq 0 ]; then
          echo "- UDN mapping requires OpenShift 4.18 or higher"
          returnCode=1
      fi
    fi

    if [[ "${options[*]}" == *"enable_all"* || "${options[*]}" == *"enable_pkt_drop"* ]]; then
      compare_versions "$version" 4.14.0
      if [ "$result" -eq 0 ]; then
          echo "- Packet drops requires OpenShift 4.14 or higher"
          returnCode=1
      fi
    fi

    return $returnCode
  fi
}

function namespaceFound() {
  # ensure namespace doesn't exist, else we should not override content
  if ${K8S_CLI_BIN} get namespace "$namespace" --ignore-not-found=true | grep -q "$namespace"; then
    return 0
  else
    return 1
  fi
}

function getSubnets() {
  declare -n sn="$1"

  # get cluster-config-v1 Configmap to retreive machine networks
  installConfig=$(${K8S_CLI_BIN} get configmap cluster-config-v1 -n kube-system -o custom-columns=":data.install-config")
  yaml="${MANIFEST_OUTPUT_PATH}/${CLUSTER_CONFIG}"
  echo "$installConfig" >"${yaml}"

  machines=$("$YQ_BIN" e -oj '.networking.machineNetwork[] | select(has("cidr")).cidr' "$yaml")
  if [ "${#machines}" -gt 0 ]; then
    sn["Machines"]=$machines
  fi

  # get OCP cluster Network to retreive pod / services / external networks
  networkConfig=$(${K8S_CLI_BIN} get network cluster -o yaml)
  yaml="${MANIFEST_OUTPUT_PATH}/${NETWORK_CONFIG}"
  echo "$networkConfig" >"${yaml}"

  pods=$("$YQ_BIN" e -oj '.spec.clusterNetwork[] | select(has("cidr")).cidr' "$yaml")
  if [ "${#pods}" -gt 0 ]; then
    sn["Pods"]=$pods
  fi

  services=$("$YQ_BIN" e -oj '.spec.serviceNetwork[] | select(.)' "$yaml")
  if [ "${#services}" -gt 0 ]; then
    sn["Services"]=$services
  fi

  if [ "${#sn[@]}" -gt 0 ]; then
    echo "Found subnets:"
    for key in "${!sn[@]}"; do
      echo "    $key: ${sn[$key]}"
    done
  else
    echo "Didn't found subnets"
  fi
}

function getNodesByLabel() {
  printf "finding nodes matching %s...\n" "$1"
  nodeStr=$("$K8S_CLI_BIN" get nodes -l "$1" -o name | tr '\n' ' ')
  IFS=' ' read -ra nodeArray <<<"$nodeStr"
  if [ "${#nodeArray[@]}" -gt 0 ]; then
    printf "%d matching nodes found: %s\n" ${#nodeArray[@]} "$nodeStr" >&2
  else
    printf "%s doesn't match any node label. Please check your --node-selector parameter\n" "$1" >&2
    exit 1
  fi
}

function applyYAML() {
  output="$1"
  if [[ "$outputYAML" == "true" ]]; then
    if [[ ! -d ${OUTPUT_PATH} ]]; then
      mkdir -p ${OUTPUT_PATH} >/dev/null
    fi

    yaml="${OUTPUT_PATH}/${YAML_OUTPUT_FILE}"
    if [ -f "$yaml" ]; then
      output="$(cat "$yaml")\n---\n$output"
    fi
    echo -e "$output" >"${yaml}"
  else
    echo "$output" | ${K8S_CLI_BIN} apply -f -
  fi
}

function setup() {
  # check for mandatory arguments
  if ! [[ $command =~ flows|packets|metrics ]]; then
    echo "invalid setup argument"
    return
  fi

  if [ -z "${YQ_BIN+x}" ]; then
    printf 'yq tools must be installed for proper usage of netobserv cli\n' >&2
    exit 1
  fi

  # load yaml files
  loadYAMLs

  # prepare manifest and validate options before any cluster changes
  if [ "$command" = "flows" ]; then
    manifest="${MANIFEST_OUTPUT_PATH}/${FLOWS_MANIFEST_FILE}"
    echo "${flowAgentYAML}" >"${manifest}"
    setCollectorPipelineConfig "$manifest"
  elif [ "$command" = "packets" ]; then
    manifest="${MANIFEST_OUTPUT_PATH}/${PACKETS_MANIFEST_FILE}"
    echo "${packetAgentYAML}" >"${manifest}"
    setCollectorPipelineConfig "$manifest"
  elif [ "$command" = "metrics" ]; then
    manifest="${MANIFEST_OUTPUT_PATH}/${METRICS_MANIFEST_FILE}"
    echo "${metricAgentYAML}" >"${manifest}"
    setMetricsPipelineConfig "$manifest"
  fi
  skipCleanup=true
  parse_args
  skipCleanup=false

  print_versions
  echo "Setting up... "

  # check cluster conditions when not outputing yaml only
  if [[ "$outputYAML" == "false" ]]; then
    if ! clusterIsReady; then
      printf 'You must be connected to cluster\n' >&2
      exit 1
    fi

    if ! checkClusterVersion; then
      printf 'Remove not compatible features and try again\n' >&2
      exit 1
    fi

    if namespaceFound; then
      printf "%s namespace already exists. Ensure someone else is not running another capture on this cluster. Else use 'oc netobserv cleanup' to remove the namespace first.\n" "$namespace" >&2
      skipCleanup="true"
      exit 1
    fi
  else
    YAML_OUTPUT_FILE="${command}_capture_${dateName}.yml"
  fi

  # Enable TLS on OpenShift for collector-based captures
  if [[ "$outputYAML" == "false" && "$isOCP" == "true" ]] && [[ "$command" = "flows" || "$command" = "packets" ]]; then
    tlsEnabled=true
    echo "OpenShift detected, enabling TLS for collector"
    enableCollectorTLS
  fi

  if [[ "$outputYAML" == "false" && -n "$nodeSelector" ]]; then
    getNodesByLabel "$nodeSelector"
  fi

  # apply yamls
  echo "creating $namespace namespace"
  applyYAML "$namespaceYAML"

  echo "creating service account"
  applyYAML "$saYAML"

  # Flows and packets captures both rely on the collector service (and its CA configmap when TLS is on)
  if [[ "$command" = "flows" || "$command" = "packets" ]]; then
    echo "creating collector service"
    applyYAML "$collectorServiceYAML"
    if [[ "$tlsEnabled" == "true" ]]; then
      echo "creating CA configmap for TLS"
      createCAConfigMap
    fi
  fi
  if [ "$command" = "flows" ]; then
    echo "creating flow-capture agents"
  elif [ "$command" = "packets" ]; then
    echo "creating packet-capture agents"
  elif [ "$command" = "metrics" ]; then
    echo "creating service monitor"
    applyYAML "$smYAML"
    echo "creating metric-capture agents:"
  fi

  yaml="$(cat "$manifest")"
  applyYAML "$yaml"
  if [[ "$outputYAML" == "false" ]]; then
    waitDaemonset
  fi
  rm -rf "${MANIFEST_OUTPUT_PATH}"
}

function follow() {
  ${K8S_CLI_BIN} logs collector -n "$namespace" -f
}

function copyOutput() {
  echo "Copying collector files to ${OUTPUT_PATH}..."
  if [[ ! -d ${OUTPUT_PATH} ]]; then
    mkdir -p ${OUTPUT_PATH} >/dev/null
  fi
  ${K8S_CLI_BIN} cp -n "$namespace" collector:output ./output
  flowFile=$(find ./output -name "*txt" | sort | tail -1)
  if [[ -n "$flowFile" ]] ; then
    buildJSON "$flowFile"
    rm "$flowFile"
  fi
}

function buildJSON() {
  file=$1
  filename=$(basename "$file")
  dirpath=$(dirname "$file")
  filenamePrefix=$(echo "$filename" | sed -E 's/(.*)\..*/\1/')
  UPDATED_JSON_FILE="$dirpath/$filenamePrefix.json"
  { 
    echo "["
    # remove last line and "," (last character) of the last flowlog for valid json
    sed '$d' "$file" | sed '$ s/.$//'
    echo "]"
  } >> "$UPDATED_JSON_FILE"
}

function deleteServiceMonitor() {
  printf "\nDeleting service monitor... "
  ${K8S_CLI_BIN} delete servicemonitor netobserv-cli -n "$namespace" --ignore-not-found=true
}

function deleteDashboardCM() {
  printf "\nDeleting dashboard configmap... "
  ${K8S_CLI_BIN} delete configmap netobserv-cli -n openshift-config-managed --ignore-not-found=true
}

function deleteDaemonset() {
  printf "\nDeleting daemonset... "
  ${K8S_CLI_BIN} delete daemonset netobserv-cli -n "$namespace" --ignore-not-found=true
}

function deletePod() {
  printf "\nDeleting pod... "
  ${K8S_CLI_BIN} delete pod collector -n "$namespace" --ignore-not-found=true
}

function deleteNamespace() {
  printf "\nDeleting namespace... "
  ${K8S_CLI_BIN} delete namespace "$namespace" --ignore-not-found=true
}

function cleanup() {
  # Trap SIGHUP to prevent premature termination when PTY closes
  trap '' HUP

  rm -rf "$MANIFEST_OUTPUT_PATH"
  if [[ "$runBackground" == "true" || "$skipCleanup" == "true" || "$outputYAML" == "true" ]]; then
    return
  fi

  # shellcheck disable=SC2034
  if clusterIsReady; then
    if [ "$captureStarted" = false ]; then
      echo "Copy skipped"
    elif [[ "$isE2E" = true || "$copy" = true ]]; then
      copyOutput
    elif [ "$copy" = "prompt" ]; then
      while true; do
        read -rp "Copy the capture output locally? [yes/no] " yn
        case $yn in
        [Yy]*)
          copyOutput
          break
          ;;
        [Nn]*)
          echo "copy skipped"
          break
          ;;
        *) echo "Please answer yes or no." ;;
        esac
      done
    fi

    printf "\nCleaning up..."
    deleteServiceMonitor
    deleteDashboardCM
    deleteDaemonset
    deletePod
    deleteNamespace
    printf "\n"
  else
    echo "Cleanup namespace skipped"
    return
  fi
}

# get current config and save it to temp file
function copyFLPConfig() {
  jsonContent=$("$YQ_BIN" e '.spec.template.spec.containers[0].env[] | select(.name=="FLP_CONFIG").value' "$1")
  # json temp file location is set as soon as this function is called
  json="${MANIFEST_OUTPUT_PATH}/${CONFIG_JSON_TEMP}"
  echo "$jsonContent" >"${json}"
}

# get network enrich stage
function getNetworkEnrichStage() {
  enrichIndex=$("$YQ_BIN" e -oj ".parameters[] | select(.name==\"enrich\") | path | .[-1]" "$json")
  enrichContent=$("$YQ_BIN" e -oj ".parameters[$enrichIndex]" "$json")
  enrichJson="${MANIFEST_OUTPUT_PATH}/enrich.json"
  echo "$enrichContent" >"${enrichJson}"
}

function overrideNetworkEnrichStage() {
  enrichJsonStr=$(cat "$enrichJson")
  "$YQ_BIN" e -oj --inplace ".parameters[$enrichIndex] = $enrichJsonStr" "$json"
}

# get prometheus stage
function getPromStage() {
  promIndex=$("$YQ_BIN" e -oj ".parameters[] | select(.name==\"prometheus\") | path | .[-1]" "$json")
  promContent=$("$YQ_BIN" e -oj ".parameters[$promIndex]" "$json")
  promJson="${MANIFEST_OUTPUT_PATH}/prom.json"
  echo "$promContent" >"${promJson}"
}

function overridePromStage() {
  promJsonStr=$(cat "$promJson")
  "$YQ_BIN" e -oj --inplace ".parameters[$promIndex] = $promJsonStr" "$json"
}

# update FLP Config
function updateFLPConfig() {
  jsonContent=$(cat "$1")
  # already escaped chars must be double-escaped
  jsonContent=${jsonContent//\\/\\\\}
  # get json as string with escaped quotes
  jsonContent=${jsonContent//\"/\\\"}

  # update FLP_CONFIG env
  "$YQ_BIN" e --inplace ".spec.template.spec.containers[0].env[] |= select(.name==\"FLP_CONFIG\").value|=\"$jsonContent\"" "$2"
}

# append a new flow filter rule to array
function addFlowFilter() {
  flowFilterJSON='
    flowFilterJSONContent
  '
  if [ -f ./res/flow-filter.json ]; then
    flowFilterJSON="$(cat ./res/flow-filter.json)"
  fi
  "$YQ_BIN" e --inplace " .spec.template.spec.containers[0].env[] |= select(.name == \"FLOW_FILTER_RULES\").value |=(fromjson | . += $flowFilterJSON | tojson)" "$1"
}

# update last flow filter of the array
function setLastFlowFilter() {
  "$YQ_BIN" e --inplace " .spec.template.spec.containers[0].env[] |= select(.name == \"FLOW_FILTER_RULES\").value |=(fromjson | .[-1].$1 = $2 | tostring)" "$3"
}

# replace the configuration in the manifest file
function edit_manifest() {
  if [ -z "${2}" ]; then
    echo "opt: $1"
  else
    echo "opt: $1, value: $2"
  fi

  if [[ $1 == "filter_"* && $1 != "filter_query" ]]; then
    # add first filter in the array
    currentFilters=$("$YQ_BIN" -r ".spec.template.spec.containers[0].env[] | select(.name == \"FLOW_FILTER_RULES\").value" "$manifest")
    if [[ $currentFilters == "[]" ]]; then
      addFlowFilter "$manifest"
    fi
  fi

  case "$1" in
  "sampling")
    "$YQ_BIN" e --inplace ".spec.template.spec.containers[0].env[] |= select(.name==\"SAMPLING\").value|=\"$2\"" "$manifest"
    ;;
  "interfaces")
    "$YQ_BIN" e --inplace ".spec.template.spec.containers[0].env[] |= select(.name==\"INTERFACES\").value|=\"$2\"" "$manifest"
    ;;
  "exclude_interfaces")
    "$YQ_BIN" e --inplace ".spec.template.spec.containers[0].env[] |= select(.name==\"EXCLUDE_INTERFACES\").value|=\"$2\"" "$manifest"
    ;;
  "pkt_drop_enable")
    "$YQ_BIN" e --inplace ".spec.template.spec.containers[0].env[] |= select(.name==\"ENABLE_PKT_DROPS\").value|=\"$2\"" "$manifest"
    ;;
  "dns_enable")
    "$YQ_BIN" e --inplace ".spec.template.spec.containers[0].env[] |= select(.name==\"ENABLE_DNS_TRACKING\").value|=\"$2\"" "$manifest"
    ;;
  "rtt_enable")
    "$YQ_BIN" e --inplace ".spec.template.spec.containers[0].env[] |= select(.name==\"ENABLE_RTT\").value|=\"$2\"" "$manifest"
    ;;
  "network_events_enable")
    "$YQ_BIN" e --inplace ".spec.template.spec.containers[0].env[] |= select(.name==\"ENABLE_NETWORK_EVENTS_MONITORING\").value|=\"$2\"" "$manifest"
    ;;
  "udn_enable")
    if [[ "$2" == "true" ]]; then
      # add udn to secondary network indexes
      copyFLPConfig "$manifest"
      getNetworkEnrichStage

      # add kubeConfig.secondaryNetworks to network
      "$YQ_BIN" e -oj --inplace ".transform.network.kubeConfig = {\"secondaryNetworks\":[{\"name\":\"ovn-kubernetes\",\"index\":{\"udn\":null}}]}" "$enrichJson"

      overrideNetworkEnrichStage
      updateFLPConfig "$json" "$manifest"
    fi
    "$YQ_BIN" e --inplace ".spec.template.spec.containers[0].env[] |= select(.name==\"ENABLE_UDN_MAPPING\").value|=\"$2\"" "$manifest"
    ;;
  "pkt_xlat_enable")
    "$YQ_BIN" e --inplace ".spec.template.spec.containers[0].env[] |= select(.name==\"ENABLE_PKT_TRANSLATION\").value|=\"$2\"" "$manifest"
    ;;
  "ipsec_enable")
    "$YQ_BIN" e --inplace ".spec.template.spec.containers[0].env[] |= select(.name==\"ENABLE_IPSEC_TRACKING\").value|=\"$2\"" "$manifest"
    ;;
  "privileged")
    "$YQ_BIN" e --inplace ".spec.template.spec.containers[0].securityContext.allowPrivilegeEscalation|=$2" "$manifest"
    "$YQ_BIN" e --inplace ".spec.template.spec.containers[0].securityContext.privileged|=$2" "$manifest"
    ;;
  "get_subnets")
    if [[ "$2" == "true" ]]; then
      declare -A subnets
      getSubnets subnets

      if [ "${#subnets[@]}" -gt 0 ]; then
        copyFLPConfig "$manifest"
        getNetworkEnrichStage

        # add rules to network
        "$YQ_BIN" e -oj --inplace ".transform.network.rules +={\"type\":\"add_subnet_label\",\"add_subnet_label\":{\"input\":\"SrcAddr\",\"output\":\"SrcSubnetLabel\"}}" "$enrichJson"
        "$YQ_BIN" e -oj --inplace ".transform.network.rules +={\"type\":\"add_subnet_label\",\"add_subnet_label\":{\"input\":\"DstAddr\",\"output\":\"DstSubnetLabel\"}}" "$enrichJson"

        # add subnetLabels to network
        "$YQ_BIN" e -oj --inplace ".transform.network.subnetLabels = []" "$enrichJson"
        for key in "${!subnets[@]}"; do
          "$YQ_BIN" e -oj --inplace ".transform.network.subnetLabels += {\"name\":\"$key\",\"cidrs\":[${subnets[$key]}]}" "$enrichJson"
        done

        overrideNetworkEnrichStage
        updateFLPConfig "$json" "$manifest"
      fi
    fi
    ;;
  "add_filter")
    addFlowFilter "$manifest"
    ;;
  "filter_direction")
    setLastFlowFilter "direction" "\"$2\"" "$manifest"
    ;;
  "filter_cidr")
    setLastFlowFilter "ip_cidr" "\"$2\"" "$manifest"
    ;;
  "filter_protocol")
    setLastFlowFilter "protocol" "\"$2\"" "$manifest"
    ;;
  "filter_sport")
    setLastFlowFilter "source_port" = "$2" "$manifest"
    ;;
  "filter_dport")
    setLastFlowFilter "destination_port" "$2" "$manifest"
    ;;
  "filter_port")
    setLastFlowFilter "port" "$2" "$manifest"
    ;;
  "filter_sport_range")
    setLastFlowFilter "source_port_range" "\"$2\"" "$manifest"
    ;;
  "filter_dport_range")
    setLastFlowFilter "destination_port_range" "\"$2\"" "$manifest"
    ;;
  "filter_port_range")
    setLastFlowFilter "port_range" "\"$2\"" "$manifest"
    ;;
  "filter_sports")
    setLastFlowFilter "source_ports" "\"$2\"" "$manifest"
    ;;
  "filter_dports")
    setLastFlowFilter "destination_ports" "\"$2\"" "$manifest"
    ;;
  "filter_ports")
    setLastFlowFilter "ports" "\"$2\"" "$manifest"
    ;;
  "filter_icmp_type")
    setLastFlowFilter "icmp_type" "$2" "$manifest"
    ;;
  "filter_icmp_code")
    setLastFlowFilter "icmp_code" "$2" "$manifest"
    ;;
  "filter_peer_ip")
    setLastFlowFilter "peer_ip" "\"$2\"" "$manifest"
    ;;
  "filter_peer_cidr")
    setLastFlowFilter "peer_cidr" "\"$2\"" "$manifest"
    ;;
  "filter_action")
    setLastFlowFilter "action" "\"$2\"" "$manifest"
    ;;
  "filter_tcp_flags")
    setLastFlowFilter "tcp_flags" "\"$2\"" "$manifest"
    ;;
  "filter_pkt_drops")
    setLastFlowFilter "drops" "$2" "$manifest"
    ;;
  "filter_query")
    copyFLPConfig "$manifest"

    # define rules from arg
    query=${2//\"/\\\"}
    rule="{\"type\":\"keep_entry_query\",\"keepEntryQuery\":\"$query\"}"

    existingFilterStage=$("$YQ_BIN" -r ".pipeline[] | select(.name == \"filter\")" "$json")
    if [[ "$existingFilterStage" == "" ]]; then
      # remove send step
      "$YQ_BIN" e -oj --inplace "del(.pipeline[] | select(.name==\"send\"))" "$json"

      # add filter param & pipeline
      "$YQ_BIN" e -oj --inplace ".parameters += {\"name\":\"filter\",\"transform\":{\"type\":\"filter\",\"filter\":{\"rules\":[$rule]}}}" "$json"
      "$YQ_BIN" e -oj --inplace ".pipeline += {\"name\":\"filter\",\"follows\":\"enrich\"}" "$json"

      # add send step back
      "$YQ_BIN" e -oj --inplace ".pipeline += {\"name\":\"send\",\"follows\":\"filter\"}" "$json"
    else 
      # add rules to existing filter param
      "$YQ_BIN" e --inplace " .parameters[] |= select(.name == \"filter\").transform.filter.rules += $rule" "$json"
    fi

    updateFLPConfig "$json" "$manifest"
    ;;
  "log_level")
    "$YQ_BIN" e --inplace ".spec.template.spec.containers[0].env[] |= select(.name==\"LOG_LEVEL\").value|=\"$2\"" "$manifest"
    ;;
  "node_selector")
    key=${2%:*}
    val=${2#*:}
    nodeSelector="$key=$val"
    "$YQ_BIN" e --inplace ".spec.template.spec.nodeSelector.\"$key\" |= \"$val\"" "$manifest"
    ;;
  "include_list")
    # restrict metrics to matching items
    copyFLPConfig "$manifest"
    getPromStage

    # list all matching metrics separated by new lines first
    filteredMetrics=""
    IFS=','
    for match in $2; do
      found=$("$YQ_BIN" -r ".encode.prom.metrics[] | select(.name | contains(\"$match\")).name" "$promJson")
      if [ "${#filteredMetrics}" -gt 0 ]; then
        filteredMetrics="${filteredMetrics}"$'\n'"${found}"
      else 
        filteredMetrics="$found"
      fi
    done

    # then, format these for YQ filter function
    echo "Matching metrics:"
    match=""
    IFS=$'\n'
    for item in $filteredMetrics; do
      echo " - $item"
      if [ "${#match}" -gt 0 ]; then
        match="$match,\"$item\""
      else 
        match="\"$item\""
      fi
    done

    "$YQ_BIN" e --inplace ".encode.prom.metrics |= filter(.name == ($match))" "$promJson"

    overridePromStage
    updateFLPConfig "$json" "$manifest"
    ;;
  esac
}

# define key and value at script level to make them available all the time
# these will be updated by parse_args first and overriden by defaultValue when needed
key=""
value=""

function defaultValue() {
  if [ "$key" == "$value" ]; then
    value="$1"
  fi
}

function waitDaemonset(){
    # In E2E test mode, skip waiting here as Go tests handle it via isDaemonsetReady()
    if [[ "$isE2E" = true ]]; then
      echo "E2E mode: Skipping daemonset wait (handled by test framework)"
      return 0
    fi

    echo "Waiting for daemonset pods to be ready..."
    # Increase timeout for CI environments where image pulls can be slow
    # 60 retries × 10 seconds = 10 minutes total
    retries=60
    while [[ $retries -ge 0 ]];do
        sleep 10
        ready=$($K8S_CLI_BIN -n "$namespace" get daemonset netobserv-cli -o jsonpath="{.status.numberReady}")
        required=$($K8S_CLI_BIN -n "$namespace" get daemonset netobserv-cli -o jsonpath="{.status.desiredNumberScheduled}")
        reasons=$($K8S_CLI_BIN get pods -n "$namespace" -o jsonpath='{.items[*].status.containerStatuses[*].state.waiting.reason}')
        IFS=" " read -r -a reasons <<< "$(echo "${reasons[@]}" | tr ' ' '\n' | sort -u | tr '\n' ' ')"
        echo "$ready/$required Ready. Reason(s): ${reasons[*]}"
        if printf '%s\0' "${reasons[@]}" | grep -Fxqz -- 'CrashLoopBackOff'; then
          break
        elif [[ $ready -eq $required ]]; then
          return
        fi
        ((retries--))
    done
    echo
    echo "ERROR: Daemonset pods failed to start within timeout"
    echo "Collecting diagnostic information..."
    echo
    echo "=== Pod Status ==="
    ${K8S_CLI_BIN} get pods -n "$namespace" -o wide
    echo
    echo "=== Pod Events ==="
    ${K8S_CLI_BIN} get events -n "$namespace" --sort-by='.lastTimestamp' | tail -20
    echo
    echo "=== Pod Descriptions ==="
    ${K8S_CLI_BIN} describe pods -n "$namespace" | grep -A 10 "Events:"
    echo
    echo "=== Daemonset Logs (if available) ==="
    ${K8S_CLI_BIN} logs daemonset/netobserv-cli -n "$namespace" --tail=50 2>&1 || echo "No logs available yet"
    echo
    exit 1
}

# Validate options and edit manifest accordingly
function parse_args() {
  # Iterate through the command-line arguments
  for option in "${options[@]}"; do
    key="${option%%=*}"
    value="${option#*=}"
    case "$key" in
    or) # Increment flow filter array
      edit_manifest "add_filter" ""
      ;;
    *background) # Run command in background
      defaultValue "true"
      if [[ "$value" == "true" || "$value" == "false" ]]; then
        runBackground="$value"
      else
        echo "invalid value for --background"
      fi
      ;;
    *yaml) # Output yamls only. Check netobserv command for implementation
      ;;
    *copy) # Copy or skip without prompt
      defaultValue "true"
      if [[ "$value" == "true" || "$value" == "false" || "$value" == "prompt" ]]; then
        copy="$value"
      else
        echo "invalid value for --copy"
      fi
      ;;
    *sampling) # ebpf sampling
      if [[ "$command" == "flows" || "$command" == "metrics" ]]; then
        edit_manifest "sampling" "$value"
      else 
        echo "--sampling is invalid option for packets"
        exit 1
      fi
      ;;
    *exclude_interfaces) # ebpf exclude interfaces
      if [[ "$command" == "flows" || "$command" == "metrics" ]]; then
        edit_manifest "exclude_interfaces" "$value"
      else 
        echo "--exclude_interfaces is invalid option for packets"
        exit 1
      fi
      ;;
    *interfaces) # Interfaces
      if [[ "$command" == "flows" || "$command" == "metrics" ]]; then
        edit_manifest "interfaces" "$value"
      else
        echo "--interfaces is invalid option for packets"
        exit 1
      fi
      ;;
    *enable_pkt_drop) # Enable packet drop
      if [[ "$command" == "flows" || "$command" == "metrics" ]]; then
        defaultValue "true"
        if [[ "$value" == "true" ]]; then
          edit_manifest "privileged" "$value"
          edit_manifest "pkt_drop_enable" "$value"
          includeList="$includeList,workload_egress_bytes_total,namespace_drop_packets_total"
        elif [[ "$value" == "false" ]]; then
          # nothing to do there
          echo
        else
          echo "invalid value for --enable_pkt_drop"
        fi
      else
        echo "--enable_pkt_drop is invalid option for packets"
        exit 1
      fi
      ;;
    *enable_dns) # Enable DNS
      if [[ "$command" == "flows" || "$command" == "metrics" ]]; then
        defaultValue "true"
        if [[ "$value" == "true" ]]; then
          edit_manifest "dns_enable" "$value"
          includeList="$includeList,namespace_dns_latency_seconds"
        elif [[ "$value" == "false" ]]; then
          # nothing to do there
          echo
        else
          echo "invalid value for --enable_dns"
        fi
      else
        echo "--enable_dns is invalid option for packets"
        exit 1
      fi
      ;;
    *enable_rtt) # Enable RTT
      if [[ "$command" == "flows" || "$command" == "metrics" ]]; then
        defaultValue "true"
        if [[ "$value" == "true" ]]; then
          edit_manifest "rtt_enable" "$value"
          includeList="$includeList,namespace_rtt_seconds"
        elif [[ "$value" == "false" ]]; then
          # nothing to do there
          echo
        else
          echo "invalid value for --enable_rtt"
        fi
      else
        echo "--enable_rtt is invalid option for packets"
        exit 1
      fi
      ;;
    *enable_network_events) # Enable Network events monitoring
      if [[ "$command" == "flows" || "$command" == "metrics" ]]; then
        defaultValue "true"
        if [[ "$value" == "true" ]]; then
          edit_manifest "privileged" "$value"
          edit_manifest "network_events_enable" "$value"
          includeList="$includeList,namespace_network_policy_events_total"
        elif [[ "$value" == "false" ]]; then
          # nothing to do there
          echo
        else
          echo "invalid value for --enable_network_events"
        fi
      else
        echo "--enable_network_events is invalid option for packets"
        exit 1
      fi
      ;;
    *enable_udn_mapping) # Enable User Defined Network mapping
      if [[ "$command" == "flows" || "$command" == "metrics" ]]; then
        defaultValue "true"
        if [[ "$value" == "true" ]]; then
          edit_manifest "privileged" "$value"
          edit_manifest "udn_enable" "$value"
        elif [[ "$value" == "false" ]]; then
          # nothing to do there
          echo
        else
          echo "invalid value for --enable_udn_mapping"
        fi
      else
        echo "--enable_udn_mapping is invalid option for packets"
        exit 1
      fi
      ;;
    *enable_pkt_translation) # Enable Packet translation
      if [[ "$command" == "flows" || "$command" == "metrics" ]]; then
        defaultValue "true"
        if [[ "$value" == "true" ]]; then
          edit_manifest "pkt_xlat_enable" "$value"
        elif [[ "$value" == "false" ]]; then
          # nothing to do there
          echo
        else
          echo "invalid value for --enable_pkt_translation"
        fi
      else
        echo "--enable_pkt_translation is invalid option for packets"
        exit 1
      fi
      ;;
    *enable_ipsec) # Enable IPSec Tracking
      if [[ "$command" == "flows" || "$command" == "metrics" ]]; then
        defaultValue "true"
        if [[ "$value" == "true" || "$value" == "false" ]]; then
          edit_manifest "ipsec_enable" "$value"
        else
          echo "invalid value for --enable_ipsec"
        fi
      else
        echo "--enable_ipsec is invalid option for packets"
        exit 1
      fi
      ;;
    *enable_all) # Enable all features
      defaultValue "true"
      if [[ "$value" == "true" ]]; then
        edit_manifest "privileged" "$value"
        edit_manifest "pkt_drop_enable" "$value"
        edit_manifest "dns_enable" "$value"
        edit_manifest "rtt_enable" "$value"
        edit_manifest "network_events_enable" "$value"
        edit_manifest "udn_enable" "$value"
        edit_manifest "pkt_xlat_enable" "$value"
        edit_manifest "ipsec_enable" "$value"
      elif [[ "$value" == "false" ]]; then
        # nothing to do there
        echo
      else
        echo "invalid value for --enable_all"
      fi
      ;;
    *privileged) # Force privileged mode
      defaultValue "true"
      if [[ "$value" == "true" ]]; then
        edit_manifest "privileged" "$value"
      elif [[ "$value" == "false" ]]; then
        # nothing to do there
        echo
      else
        echo "invalid value for --privileged"
      fi
      ;;
    *direction) # Configure filter direction
      if [[ "$value" == "Ingress" || "$value" == "Egress" ]]; then
        edit_manifest "filter_direction" "$value"
      else
        echo "invalid value for --direction"
      fi
      ;;
    *peer_cidr) # Peer CIDR
      edit_manifest "filter_peer_cidr" "$value"
      ;;
    *cidr) # Configure flow CIDR
      edit_manifest "filter_cidr" "$value"
      ;;
    *protocol) # Configure filter protocol
      if [[ "$value" == "TCP" || "$value" == "UDP" || "$value" == "SCTP" || "$value" == "ICMP" || "$value" == "ICMPv6" ]]; then
        edit_manifest "filter_protocol" "$value"
      else
        echo "invalid value for --protocol"
      fi
      ;;
    *sport) # Configure filter source port
      edit_manifest "filter_sport" "$value"
      ;;
    *dport) # Configure filter destination port
      edit_manifest "filter_dport" "$value"
      ;;
    *port) # Configure filter port
      edit_manifest "filter_port" "$value"
      ;;
    *sport_range) # Configure filter source port range
      edit_manifest "filter_sport_range" "$value"
      ;;
    *dport_range) # Configure filter destination port range
      edit_manifest "filter_dport_range" "$value"
      ;;
    *port_range) # Configure filter port range
      edit_manifest "filter_port_range" "$value"
      ;;
    *sports) # Configure filter source two ports using ","
      edit_manifest "filter_sports" "$value"
      ;;
    *dports) # Configure filter destination two ports using ","
      edit_manifest "filter_dports" "$value"
      ;;
    *ports) # Configure filter on two ports usig "," can either be srcport or dstport
      edit_manifest "filter_ports" "$value"
      ;;
    *tcp_flags) # Configure filter TCP flags
      if [[ "$value" == "SYN" || "$value" == "SYN-ACK" || "$value" == "ACK" || "$value" == "FIN" || "$value" == "RST" || "$value" == "FIN-ACK" || "$value" == "RST-ACK" || "$value" == "PSH" || "$value" == "URG" || "$value" == "ECE" || "$value" == "CWR" ]]; then
        edit_manifest "filter_tcp_flags" "$value"
      else
        echo "invalid value for --tcp_flags"
      fi
      ;;
    *drops) # Filter packet drops
      defaultValue "true"
      if [[ "$value" == "true" ]]; then
        edit_manifest "privileged" "$value"
        edit_manifest "pkt_drop_enable" "$value"
        edit_manifest "filter_pkt_drops" "$value"
      elif [[ "$value" == "false" ]]; then
        # nothing to do there
        echo
      else
        echo "invalid value for --drops"
      fi
      ;;
    *query) # Filter using a custom query
      if [[ "$value" != "" ]]; then
        edit_manifest "filter_query" "$value"
      else
        echo "missing value for --query"
        exit 1
      fi
      ;;
    *icmp_type) # ICMP type
      edit_manifest "filter_icmp_type" "$value"
      ;;
    *icmp_code) # ICMP code
      edit_manifest "filter_icmp_code" "$value"
      ;;
    *peer_ip) # Peer IP
      edit_manifest "filter_peer_ip" "$value"
      ;;
    *action) # Filter action
      if [[ "$value" == "Accept" || "$value" == "Reject" ]]; then
        edit_manifest "filter_action" "$value"
      else
        echo "invalid value for --action"
      fi
      ;;
    *log-level) # Log level
      if [[ "$value" == "trace" || "$value" == "debug" || "$value" == "info" || "$value" == "warn" || "$value" == "error" || "$value" == "fatal" || "$value" == "panic" ]]; then
        edit_manifest "log_level" "$value"
        logLevel=$value
        filter=${filter/$key=$logLevel/}
      else
        echo "invalid value for --log-level"
      fi
      ;;
    *max-time) # Max time
      maxTime=$value
      filter=${filter/$key=$maxTime/}
      ;;
    *max-bytes) # Max bytes
      if [[ "$command" == "flows" || "$command" == "packets" ]]; then
        maxBytes=$value
        filter=${filter/$key=$maxBytes/}
      else 
        echo "--max-bytes is invalid option for metrics"
        exit 1
      fi
      ;;
    *node-selector) # Node selector
      if [[ $value == *":"* ]]; then
        edit_manifest "node_selector" "$value"
      else
        echo "invalid value for --node-selector. Use --node-selector=key:val instead."
        exit 1
      fi
      ;;
    *get-subnets) # Get subnets
      defaultValue "true"
      if [[ "$value" == "true" || "$value" == "false" ]]; then
        edit_manifest "get_subnets" "$value"
      else
        echo "invalid value for --get-subnets"
      fi
      ;;
    *include_list) # Restrict metrics capture
      if [[ "$command" == "metrics" ]]; then
        includeList="$value"
      else
        echo "--include_list is invalid option for $command"
        exit 1
      fi
      ;;
    *) # Invalid option
      echo "Invalid option: $key" >&2
      exit 1
      ;;
    esac
  done

  # avoid packet capture without filters
  if [[ "$command" = "packets" ]]; then
    currentFilters=$("$YQ_BIN" -r ".spec.template.spec.containers[0].env[] | select(.name == \"FLOW_FILTER_RULES\").value" "$manifest")
    if [[ $currentFilters == "[]" ]]; then
      echo
      echo "Error: At least one eBPF filter must be set for packet capture to avoid high resource consumption."
      echo "Use netobserv packets help to list filters"
      echo
      exit 1
    fi
  elif [[ "$command" = "metrics" ]]; then
    # always restrict generated metrics
    edit_manifest "include_list" "$includeList"
  fi
}
