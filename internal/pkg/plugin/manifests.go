package plugin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/netobserv/network-observability-cli/res"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	kyaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"
)

// Overridden by the release build, with runtime environment overrides in NewCommand.
var CollectorImage = "quay.io/netobserv/network-observability-cli:main"
var AgentImage = "quay.io/netobserv/netobserv-ebpf-agent:main"
var PullPolicy = "Always"

type manifests struct {
	objects []*unstructured.Unstructured
	agent   *appsv1.DaemonSet
	pod     *corev1.Pod
}

func resourceObjects(name string, o *options) ([]*unstructured.Unstructured, error) {
	b, err := res.Files.ReadFile(name)
	if err != nil {
		return nil, err
	}
	s := strings.NewReplacer("{{NAME}}", o.namespace, "{{NAMESPACE}}", o.namespace, "{{AGENT_IMAGE_URL}}", envDefault("NETOBSERV_AGENT_IMAGE", AgentImage)).Replace(string(b))
	decoder := kyaml.NewYAMLOrJSONDecoder(strings.NewReader(s), 4096)
	var result []*unstructured.Unstructured
	for {
		obj := &unstructured.Unstructured{}
		err = decoder.Decode(&obj.Object)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("decode %s: %w", name, err)
		}
		if len(obj.Object) > 0 {
			result = append(result, obj)
		}
	}
	return result, nil
}

func buildManifests(o *options, openshift bool, subnets []any) (*manifests, error) {
	m := &manifests{}
	tls := openshift && o.mode != "metrics"
	if err := m.loadResources(o, openshift, tls); err != nil {
		return nil, err
	}
	if err := m.buildAgent(o); err != nil {
		return nil, err
	}
	pipeline, err := buildPipeline(o, tls, subnets)
	if err != nil {
		return nil, err
	}
	setEnv(&m.agent.Spec.Template.Spec.Containers[0], "FLP_CONFIG", pipeline)
	m.buildCollector(o, tls)
	if err := m.addKeylog(o); err != nil {
		return nil, err
	}
	return m, nil
}
func (m *manifests) loadResources(o *options, openshift, tls bool) error {
	files := []string{"namespace.yml", "service-account.yml"}
	if o.mode == "metrics" {
		files = append(files, "service-monitor.yml")
	} else {
		files = append(files, "collector-service.yml")
	}
	for _, file := range files {
		objects, err := resourceObjects(file, o)
		if err != nil {
			return err
		}
		for _, obj := range objects {
			if obj.GetKind() == "SecurityContextConstraints" && !openshift && !o.yaml {
				continue
			}
			if obj.GetKind() == "ConfigMap" && obj.GetNamespace() == "openshift-config-managed" && obj.GetName() == "netobserv-cli" {
				obj.SetName(o.namespace)
			}
			switch obj.GetKind() {
			case "ClusterRole", "ClusterRoleBinding", "SecurityContextConstraints":
				obj.SetNamespace("")
				obj.SetName(strings.Replace(obj.GetName(), "netobserv-cli", o.namespace, 1))
			}
			if obj.GetKind() == "ClusterRoleBinding" {
				role, _, _ := unstructured.NestedString(obj.Object, "roleRef", "name")
				if strings.HasPrefix(role, "netobserv-cli") {
					_ = unstructured.SetNestedField(obj.Object, strings.Replace(role, "netobserv-cli", o.namespace, 1), "roleRef", "name")
				}
			}
			if obj.GetKind() == "SecurityContextConstraints" {
				_ = unstructured.SetNestedStringSlice(obj.Object, []string{"system:serviceaccount:" + o.namespace + ":netobserv-cli"}, "users")
			}
			if tls && obj.GetKind() == "Service" {
				obj.SetAnnotations(map[string]string{"service.beta.openshift.io/serving-cert-secret-name": "collector-tls"})
			}
			m.objects = append(m.objects, obj)
		}
	}
	if tls {
		m.objects = append(m.objects, &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"name": "collector-ca", "namespace": o.namespace, "annotations": map[string]any{"service.beta.openshift.io/inject-cabundle": "true"}}}})
	}
	return nil
}
func (m *manifests) buildAgent(o *options) error {
	file := map[string]string{"flows": "flow-capture.yml", "packets": "packet-capture.yml", "metrics": "metric-capture.yml"}[o.mode]
	agents, err := resourceObjects(file, o)
	if err != nil {
		return err
	}
	m.agent = &appsv1.DaemonSet{}
	if err = runtime.DefaultUnstructuredConverter.FromUnstructured(agents[0].Object, m.agent); err != nil {
		return err
	}
	c := &m.agent.Spec.Template.Spec.Containers[0]
	// PullPolicy configures the collector image (preloaded in Kind tests).
	// Agents keep their template policy because their image is pulled separately.
	setEnv(c, "LOG_LEVEL", o.logLevel)
	for _, v := range o.values {
		switch v.key {
		case "sampling", "interfaces", "exclude_interfaces":
			setEnv(c, strings.ToUpper(v.key), v.value)
		case "node-selector":
			k, val, _ := strings.Cut(v.value, ":")
			if m.agent.Spec.Template.Spec.NodeSelector == nil {
				m.agent.Spec.Template.Spec.NodeSelector = map[string]string{}
			}
			m.agent.Spec.Template.Spec.NodeSelector[k] = val
		}
	}
	configurePlaintextAgent(&m.agent.Spec.Template.Spec, o)
	privileged := o.enabled("enable_openssl") || o.enabled("privileged") || o.enabled("enable_pkt_drop") || o.enabled("enable_network_events") || o.enabled("enable_udn_mapping") || o.enabled("drops")
	c.SecurityContext.Privileged = &privileged
	c.SecurityContext.AllowPrivilegeEscalation = &privileged
	for key, env := range featureEnv {
		if o.enabled(key) || key == "enable_pkt_drop" && o.enabled("drops") {
			setEnv(c, env, "true")
		}
	}
	filters := o.filters
	if filters == nil {
		filters = []map[string]any{}
	}
	b, err := json.Marshal(filters)
	if err != nil {
		return err
	}
	setEnv(c, "FLOW_FILTER_RULES", string(b))
	return nil
}
func (m *manifests) buildCollector(o *options, tls bool) {
	c := &m.agent.Spec.Template.Spec.Containers[0]
	image := envDefault("NETOBSERV_COLLECTOR_IMAGE", CollectorImage)
	m.pod = &corev1.Pod{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"}, ObjectMeta: metav1.ObjectMeta{Name: "collector", Namespace: o.namespace, Labels: map[string]string{"run": "collector"}}, Spec: corev1.PodSpec{ServiceAccountName: "netobserv-cli", RestartPolicy: corev1.RestartPolicyNever, Containers: []corev1.Container{{Name: "collector", Image: image, ImagePullPolicy: corev1.PullPolicy(PullPolicy), Command: []string{"sleep", "infinity"}}}}}
	if o.background || o.yaml {
		// Arguments are passed separately to bash; user input is never interpreted as shell code.
		m.pod.Spec.Containers[0].Command = append([]string{"bash", "-c", `"$@" && sleep infinity`, "--"}, o.collectorCommand()...)
	}
	if tls {
		optional := false
		envFrom := corev1.EnvFromSource{ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "collector-tls-config"}, Optional: &optional}}
		c.EnvFrom = append(c.EnvFrom, envFrom)
		c.VolumeMounts = append(c.VolumeMounts, corev1.VolumeMount{Name: "collector-ca", MountPath: "/etc/collector-ca", ReadOnly: true})
		m.agent.Spec.Template.Spec.Volumes = append(m.agent.Spec.Template.Spec.Volumes, corev1.Volume{Name: "collector-ca", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "collector-ca"}}}})
		m.pod.Spec.InitContainers = []corev1.Container{{Name: "resolve-tls", Image: image, ImagePullPolicy: corev1.PullPolicy(PullPolicy), Command: []string{"/network-observability-cli", "resolve-tls", "--namespace", o.namespace}}}
		m.pod.Spec.Containers[0].EnvFrom = []corev1.EnvFromSource{envFrom}
		m.pod.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{{Name: "collector-tls", MountPath: "/etc/collector-tls", ReadOnly: true}}
		m.pod.Spec.Volumes = []corev1.Volume{{Name: "collector-tls", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "collector-tls"}}}}
	}
}

func setEnv(c *corev1.Container, name, value string) {
	for i := range c.Env {
		if c.Env[i].Name == name {
			c.Env[i].Value = value
			return
		}
	}

}
func (o *options) collectorCommand() []string {
	args := []string{"/network-observability-cli", "get-" + o.mode, "--loglevel", o.logLevel, "--maxtime", o.maxTime.String(), "--namespace", o.namespace}
	if o.mode != "metrics" {
		args = append(args, "--maxbytes", fmt.Sprint(o.maxBytes))
	}
	for _, v := range o.values {
		if v.key == "tls-keylog" {
			args = append(args, "--tls-keylog", collectorKeylogPath)
			break
		}
	}
	opts := append([]string(nil), o.raw...)
	if (o.yaml || o.headless) && !o.background {
		opts = append(opts, "background=true")
	}

	return append(args, "--options", strings.Join(opts, "|"))
}
func (m *manifests) writeYAML(w io.Writer) error {
	objects := []any{}
	for _, obj := range m.objects {
		objects = append(objects, obj.Object)
	}
	objects = append(objects, m.agent)
	for _, obj := range objects {
		b, err := yaml.Marshal(obj)
		if err != nil {
			return err
		}
		if _, err = fmt.Fprintf(w, "---\n%s", b); err != nil {
			return err
		}
	}
	return nil
}

func buildPipeline(o *options, tls bool, subnets []any) (string, error) {
	pipelineName := "collector-pipeline-config.json"
	if o.mode == "metrics" {
		pipelineName = "metrics-pipeline-config.json"
	}
	b, err := res.Files.ReadFile(pipelineName)
	if err != nil {
		return "", err
	}
	var pipeline map[string]any
	if err = json.Unmarshal(bytes.ReplaceAll(b, []byte("{{TARGET_HOST}}"), []byte("collector."+o.namespace+".svc.cluster.local")), &pipeline); err != nil {
		return "", err
	}
	parameters := pipeline["parameters"].([]any)
	for _, p := range parameters {
		param := p.(map[string]any)
		if param["name"] == "enrich" {
			enrichNetwork(param, o, subnets)
		}
		if param["name"] == "send" && tls {
			param["write"].(map[string]any)["grpc"].(map[string]any)["tls"] = map[string]any{"caCertPath": "/etc/collector-ca/service-ca.crt"}
		}
		if enc, ok := param["encode"].(map[string]any); ok {
			if prom, ok := enc["prom"].(map[string]any); ok {
				selectMetrics(prom, o)
			}
		}
	}
	addQueries(pipeline, o)
	b, err = json.Marshal(pipeline)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func enrichNetwork(param map[string]any, o *options, subnets []any) {
	network := param["transform"].(map[string]any)["network"].(map[string]any)
	if o.enabled("enable_udn_mapping") {
		network["kubeConfig"] = map[string]any{"secondaryNetworks": []any{map[string]any{"name": "ovn-kubernetes", "index": map[string]any{"udn": nil}}}}
	}
	if len(subnets) > 0 {
		network["subnetLabels"] = subnets
		rules := network["rules"].([]any)
		for _, side := range []string{"Src", "Dst"} {
			rules = append(rules, map[string]any{"type": "add_subnet_label", "add_subnet_label": map[string]any{"input": side + "Addr", "output": side + "SubnetLabel"}})
		}
		network["rules"] = rules
	}
}

func selectMetrics(prom map[string]any, o *options) {
	include := "namespace_flows_total,node_ingress_bytes_total,node_egress_bytes_total,workload_ingress_bytes_total"
	extras := map[string]string{"enable_pkt_drop": "workload_egress_bytes_total,namespace_drop_packets_total", "enable_dns": "namespace_dns_latency_seconds", "enable_rtt": "namespace_rtt_seconds", "enable_network_events": "namespace_network_policy_events_total"}
	for _, v := range o.values {
		if v.key == "include_list" {
			include = v.value
		}
		if extra := extras[v.key]; extra != "" && v.value == "true" {
			include += "," + extra
		}
	}
	selected := []any{}
	for _, metric := range prom["metrics"].([]any) {
		for _, match := range strings.Split(include, ",") {
			if match != "" && strings.Contains(metric.(map[string]any)["name"].(string), match) {
				selected = append(selected, metric)
				break
			}
		}
	}
	prom["metrics"] = selected
}

func addQueries(pipeline map[string]any, o *options) {
	rules := []any{}
	for _, v := range o.values {
		if v.key == "query" {
			rules = append(rules, map[string]any{"type": "keep_entry_query", "keepEntryQuery": v.value})
		}
	}
	if len(rules) > 0 {
		pipeline["parameters"] = append(pipeline["parameters"].([]any), map[string]any{"name": "filter", "transform": map[string]any{"type": "filter", "filter": map[string]any{"rules": rules}}})
		stages := pipeline["pipeline"].([]any)
		ordered := []any{}
		for _, s := range stages {
			stage := s.(map[string]any)
			if stage["follows"] == "enrich" {
				ordered = append(ordered, map[string]any{"name": "filter", "follows": "enrich"})
				stage["follows"] = "filter"
			}
			ordered = append(ordered, stage)
		}
		pipeline["pipeline"] = ordered
	}
}
