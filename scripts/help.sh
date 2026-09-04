#!/usr/bin/env bash

# metrics includeList
includeList="namespace_flows_total,node_ingress_bytes_total,node_egress_bytes_total,workload_ingress_bytes_total"

_BOLD_CYAN=$(printf '\033[1;36m')
_RESET=$(printf '\033[0m')

# Extract keywords from args for help highlighting
# e.g. "--port=8080 --drops --help" → "--port" "--drops"
get_help_keywords() {
  for arg in "$@"; do
    case "$arg" in
      *help|or) ;;
      --*=*) printf '%s\n' "${arg%%=*}" ;;
      *) printf '%s\n' "$arg" ;;
    esac
  done
}

# Pipe help text through this to highlight keywords
# Usage: some_usage_fn | highlight_keywords --port --drops
highlight_keywords() {
  if [ $# -eq 0 ]; then
    cat
    return
  fi

  local sed_args=()
  for keyword in "$@"; do
    keyword="${keyword#--}"
    sed_args+=(-e "s|${keyword}|${_BOLD_CYAN}&${_RESET}|g")
  done

  sed "${sed_args[@]}"
}

# display main help
function help {
  echo
  echo "NetObserv allows you to capture flows, packets and metrics from your cluster."
  echo "Find more information at: https://github.com/netobserv/network-observability-cli/"
  echo
  echo "Syntax: netobserv [flows|packets|metrics|follow|stop|copy|cleanup|version] [options]"
  echo
  echo "Main commands:"
  echo "  flows      Capture flows information in JSON format using collector pod."
  echo "  metrics    Capture metrics information in Prometheus using a ServiceMonitor (OCP cluster only)."
  echo "  packets    Capture packets information in pcap format using collector pod."
  echo
  echo "Extra commands:"
  echo "  cleanup    Remove netobserv components and configurations."
  echo "  copy       Copy collector generated files locally."
  echo "  follow     Follow collector logs when running in background."
  echo "  stop       Stop collection by removing agent daemonset."
  echo "  version    Print software version."
  echo
  echo "Use --help with any command for more details (e.g. netobserv flows --help)"
  echo
  echo "Flow capture examples:"
  flows_examples
  echo
  echo "Packet capture examples:"
  packets_examples
  echo
  echo "Metrics capture examples:"
  metrics_examples
  echo
}

# flows examples
function flows_examples {
  echo "  Capture dropped flows on all nodes:"
  echo "    netobserv flows --drops"
  echo "  Capture flows in the background, for maximum 15 minutes, TCP on 8080 or UDP, and copy output locally:"
  echo "    netobserv flows --background --max-time=15m --protocol=TCP --port=8080 or --protocol=UDP"
  echo "  Capture flows from any namespace starting with 'app-': (for --query doc, see also: https://github.com/netobserv/flowlogs-pipeline/blob/main/docs/filtering.md)"
  echo "    netobserv flows --query='SrcK8S_Namespace=~\"app-.*\"'"
  echo "  Capture flows from/to a specific pod pattern on a specific node:"
  echo "    netobserv flows --node-selector=kubernetes.io/hostname:my-node --query='SrcK8S_Name=~\".*my-pod.*\" or DstK8S_Name=~\".*my-pod.*\"'"
}

# packets examples
function packets_examples {
  echo "  Capture packets on port 8080:"
  echo "    netobserv packets --port=8080"
  echo "  Capture HTTPS with OpenSSL plaintext via libssl uprobes:"
  echo "    netobserv packets --port=8443 --enable_openssl --peer_ip=<pod-ip> --privileged --background"
  echo "    # test workload: examples/openssl-test-pod/"
  echo "    # readable output: output/plaintext/<timestamp>.jsonl (PlaintextDisplay field)"
  echo "  Capture with Wireshark key log embedded in pcapng:"
  echo "    netobserv packets --port=443 --tls-keylog=/path/to/keylog.txt"
  echo "  Capture packets on specific nodes (labeled with 'netobserv=true') and port, for a maximum of 100MB:"
  echo "    netobserv packets --node-selector=netobserv:true --port=80 --max-bytes=100000000"
}

# metrics examples
function metrics_examples {
  echo "  Capture default cluster metrics including packet drop, dns, rtt, network events packet translation and UDN mapping features:"
  echo "    netobserv metrics --enable_all"
  echo "  Capture node and namespace drop metrics, based on keywords include list:"
  echo "    netobserv metrics --drops --include_list=node,namespace"
  echo "  Capture metrics in the background for 1 day:"
  echo "    netobserv metrics --background --max-time=24h"
  echo "    Then open the URL provided by the command to visualize the netobserv-cli dashboard anytime during or after the run."
  echo
}

# display version
function version {
  echo "NetObserv CLI version $1"
}

# agent / flp features
function features_usage {
  echo "  --enable_all:                 enable all eBPF features                              (default: false)"
  echo "  --enable_dns:                 enable DNS tracking                                   (default: false)"
  echo "  --enable_ipsec:               enable IPsec tracking                                 (default: false)"
  echo "  --enable_network_events:      enable network events monitoring                      (default: false)"
  echo "  --enable_pkt_translation:     enable packet translation                             (default: false)"
  echo "  --enable_pkt_drop:            enable packet drop                                    (default: false)"
  echo "  --enable_rtt:                 enable RTT tracking                                   (default: false)"
  echo "  --enable_udn_mapping:         enable User Defined Network mapping                   (default: false)"
  echo "  --get-subnets:                get subnets information                               (default: false)"
  echo "  --privileged:                 force eBPF agent privileged mode                      (default: auto)"
  echo "  --sampling:                   packets sampling interval                             (default: 1)"
}

# flow and packets collector options
function flowsAndPackets_collector_usage {
  echo "  --background:                 run in background                                     (default: false)"
  echo "  --copy:                       copy the output files locally                         (default: prompt)"
  echo "  --log-level:                  components logs                                       (default: info)"
  echo "  --max-time:                   maximum capture time                                  (default: 5m)"
  echo "  --max-bytes:                  maximum capture bytes                                 (default: 50000000 = 50MB)"
}

function packets_tls_usage {
  echo "  --enable_openssl:             capture TLS plaintext via OpenSSL uprobes             (default: false, requires --privileged; recommended: --peer_ip --port)"
  echo "  --tls_plaintext_min_bytes:    drop TLS plaintext events shorter than N bytes         (default: 0, agent env TLS_PLAINTEXT_MIN_BYTES)"
  echo "  --tls_plaintext_preview_bytes: PlaintextPreview length (0 = full captured payload)   (default: 256)"
  echo "  --tls-keylog:                 path to SSLKEYLOGFILE for pcapng decryption           (collector flag)"
}

# fmetrics collector options
function metrics_collector_usage {
  echo "  --background:                 run in background                                     (default: false)"
  echo "  --log-level:                  components logs                                       (default: info)"
  echo "  --max-time:                   maximum capture time                                  (default: 1h)"
}

# script options
function script_usage {
  echo "  --yaml:                       generate YAML without applying it                     (default: false)"
}

# agent selector / filters
function filters_usage {
  echo "  --action:                     filter action                                         (default: Accept)"
  echo "  --cidr:                       filter CIDR                                           (default: 0.0.0.0/0)"
  echo "  --direction:                  filter direction                                      (default: n/a)"
  echo "  --dport:                      filter destination port                               (default: n/a)"
  echo "  --dport_range:                filter destination port range                         (default: n/a)"
  echo "  --dports:                     filter on either of two destination ports             (default: n/a)"
  echo "  --drops:                      filter flows with only dropped packets                (default: false)"
  echo "  --icmp_code:                  filter ICMP code                                      (default: n/a)"
  echo "  --icmp_type:                  filter ICMP type                                      (default: n/a)"
  echo "  --node-selector:              capture on specific nodes                             (default: n/a)"
  echo "  --peer_ip:                    scope which processes get TLS hooks; filter peer IP     (default: n/a; recommended with TLS flags)"
  echo "  --peer_cidr:                  filter peer CIDR                                      (default: n/a)"
  echo "  --port_range:                 filter port range                                     (default: n/a)"
  echo "  --port:                       filter exported plaintext and wire capture            (default: n/a; recommended with TLS flags)"
  echo "  --ports:                      filter on either of two ports                         (default: n/a)"
  echo "  --protocol:                   filter protocol                                       (default: n/a)"
  echo "  --query:                      filter flows using a custom query                     (default: n/a)"
  echo "  --sport_range:                filter source port range                              (default: n/a)"
  echo "  --sport:                      filter source port                                    (default: n/a)"
  echo "  --sports:                     filter on either of two source ports                  (default: n/a)"
  echo "  --tcp_flags:                  filter TCP flags                                      (default: n/a)"
}

# specific filters for flows and metrics
function flowsAndMetrics_filters_usage {
  echo "  --interfaces:                 list of interfaces to monitor, comma separated        (default: n/a)"
  echo "  --exclude_interfaces:         list of interfaces to exclude, comma separated        (default: lo)"
}

# specific filters for metrics
function metrics_options {
  echo "  --include_list:               list of metric names to generate, comma separated     (default: $includeList)"
}

function flows_usage {
  echo
  echo "NetObserv allows you to capture flows from your cluster."
  echo "Find more information at: https://github.com/netobserv/network-observability-cli/"
  echo
  echo "Syntax: netobserv flows [options]"
  echo
  echo "Features:"
  features_usage
  echo
  echo "Filters:"
  filters_usage
  flowsAndMetrics_filters_usage
  echo
  echo "Options:"
  flowsAndPackets_collector_usage
  script_usage
  echo
  echo "Examples:"
  flows_examples
}

function packets_usage {
  echo
  echo "NetObserv allows you to capture packets from your cluster."
  echo "Find more information at: https://github.com/netobserv/network-observability-cli/"
  echo
  echo "Syntax: netobserv packets [options]"
  echo
  echo "Filters:"
  filters_usage
  echo
  echo "Options:"
  flowsAndPackets_collector_usage
  packets_tls_usage
  script_usage
  echo
  echo "Examples:"
  packets_examples
}

function metrics_usage {
  echo
  echo "NetObserv allows you to capture metrics on your OCP cluster."
  echo "Find more information at: https://github.com/netobserv/network-observability-cli/"
  echo
  echo "Syntax: netobserv metrics [options]"
  echo
  echo "Features:"
  features_usage
  echo
  echo "Filters:"
  filters_usage
  flowsAndMetrics_filters_usage
  echo
  echo "Options:"
  metrics_collector_usage
  script_usage
  metrics_options
  echo
  echo "Examples:"
  metrics_examples
  echo
  echo "More information, with full list of available metrics: https://github.com/netobserv/network-observability-operator/blob/main/docs/Metrics.md"
}

function follow_usage {
  echo
  echo "NetObserv allows you to capture flows and packets asynchronously using the --background option."
  echo "While the capture is running in background, you can connect to the collector pod to see the progression using the follow command."
  echo "Find more information at: https://github.com/netobserv/network-observability-cli/"
  echo
  echo "Syntax: netobserv follow"
  echo
}

function stop_usage {
  echo
  echo "NetObserv allows you to stop the collection and keep collector or dashboard for post analysis."
  echo "While the capture is running, use the stop command to remove the eBPF agents."
  echo "Find more information at: https://github.com/netobserv/network-observability-cli/"
  echo
  echo "Syntax: netobserv stop"
  echo
}

function copy_usage {
  echo
  echo "NetObserv allows you to copy locally the captured flows or packets from the collector pod."
  echo "While the collector is running, use the copy command to copy the output file(s)."
  echo "To avoid modifications during the copy, it's recommended to stop the capture"
  echo "Find more information at: https://github.com/netobserv/network-observability-cli/"
  echo
  echo "Syntax: netobserv copy"
  echo
}

function cleanup_usage {
  echo
  echo "NetObserv may require manual cleanup in some cases such as after a background run or in case of failure."
  echo "Use the cleanup command to remove all the netobserv CLI components."
  echo "Find more information at: https://github.com/netobserv/network-observability-cli/"
  echo
  echo "Syntax: netobserv cleanup"
  echo
}
