package manifests

import (
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
)

// CaptureKind selects which res/*-capture.yml DaemonSet template to build. The three
// templates differ in which env vars/ports/volumes they ship (e.g. only packet-capture.yml
// has ENABLE_PCA/TLS_PLAINTEXT_* defaults, only metric-capture.yml exposes a prometheus port) —
// unlike bash's parse_args, which validated flag-vs-command compatibility at runtime, the Cobra
// command tree (internal/plugin) only defines each flag on the commands it applies to, so
// internal/plugin/manifests/agent never needs to re-validate "is this option valid for packets?".
type CaptureKind string

const (
	Flows   CaptureKind = "flows"
	Packets CaptureKind = "packets"
	Metrics CaptureKind = "metrics"
)

// HasCollector reports whether the capture sends data to the ephemeral
// collector Service. Metrics are scraped directly by the monitoring stack.
func (k CaptureKind) HasCollector() bool {
	return k == Flows || k == Packets
}

// Singular is used for human-facing progress messages.
func (k CaptureKind) Singular() string {
	return strings.TrimSuffix(string(k), "s")
}

func (k CaptureKind) templateFile() (string, error) {
	switch k {
	case Flows:
		return "flow-capture.yml", nil
	case Packets:
		return "packet-capture.yml", nil
	case Metrics:
		return "metric-capture.yml", nil
	default:
		return "", fmt.Errorf("unknown capture kind %q", k)
	}
}

// BuildDaemonSet decodes the DaemonSet template for kind, substituting the namespace and agent
// image. Replaces loadYAMLs' flowAgentYAML/packetAgentYAML/metricAgentYAML templating. The
// returned object still has placeholder env values (FLP_CONFIG: "{}", FLOW_FILTER_RULES: "[]")
// — internal/plugin/manifests/agent.Builder fills those in and applies every --flag mutation.
func BuildDaemonSet(kind CaptureKind, namespace, agentImage string) (*appsv1.DaemonSet, error) {
	file, err := kind.templateFile()
	if err != nil {
		return nil, err
	}
	data, err := readTemplate(file, map[string]string{
		"{{NAMESPACE}}":       namespace,
		"{{AGENT_IMAGE_URL}}": agentImage,
	})
	if err != nil {
		return nil, err
	}
	ds := &appsv1.DaemonSet{}
	if err := decodeInto(data, ds); err != nil {
		return nil, err
	}
	return ds, nil
}
