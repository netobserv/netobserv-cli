// Package agent mutates a capture DaemonSet the way scripts/functions.sh's edit_manifest
// and parse_args did with yq, except every mutation is a typed Go method instead of a jsonpath
// expression string: env vars are set through real []corev1.EnvVar manipulation, and the two
// JSON blobs those env vars carry (FLP_CONFIG, FLOW_FILTER_RULES) are decoded into the upstream
// typed structs their own producers already define — github.com/netobserv/flowlogs-pipeline's
// pkg/config.Root/pkg/api stage types, and github.com/netobserv/netobserv-ebpf-agent's
// pkg/config.FlowFilter — rather than treated as opaque strings edited by jsonpath.
package agent

import (
	"encoding/json"
	"fmt"

	flpconfig "github.com/netobserv/flowlogs-pipeline/pkg/config"
	agentconfig "github.com/netobserv/netobserv-ebpf-agent/pkg/config"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// Env var names the DaemonSet templates (res/*-capture.yml) define and this package mutates.
const (
	envSampling                 = "SAMPLING"
	envInterfaces               = "INTERFACES"
	envExcludeInterfaces        = "EXCLUDE_INTERFACES"
	envPktDrops                 = "ENABLE_PKT_DROPS"
	envDNSTracking              = "ENABLE_DNS_TRACKING"
	envRTT                      = "ENABLE_RTT"
	envNetworkEvents            = "ENABLE_NETWORK_EVENTS_MONITORING"
	envUDNMapping               = "ENABLE_UDN_MAPPING"
	envPktTranslation           = "ENABLE_PKT_TRANSLATION"
	envIPSecTracking            = "ENABLE_IPSEC_TRACKING"
	envOpenSSLTracking          = "ENABLE_OPENSSL_TRACKING"
	envTLSPlaintextMinBytes     = "TLS_PLAINTEXT_MIN_BYTES"
	envTLSPlaintextPreviewBytes = "TLS_PLAINTEXT_PREVIEW_BYTES"
	envTLSPlaintextAllowlist    = "TLS_PLAINTEXT_PROCESS_ALLOWLIST"
	envLogLevel                 = "LOG_LEVEL"
	envFLPConfig                = "FLP_CONFIG"
	envFlowFilterRules          = "FLOW_FILTER_RULES"
)

// Builder wraps a capture DaemonSet together with the decoded contents of its two JSON-in-env
// blobs, so every --flag mutation can be expressed as a typed field assignment. Build()
// re-serializes the pipeline/filters back into the DaemonSet's env vars.
type Builder struct {
	ds       *appsv1.DaemonSet
	pipeline *flpconfig.Root
	filters  []agentconfig.FlowFilter
}

// New wraps a DaemonSet freshly built by internal/plugin/manifests.BuildDaemonSet (whose
// FLP_CONFIG/FLOW_FILTER_RULES env vars are still the "{}"/"[]" placeholders) together with the
// FLP pipeline config to populate FLP_CONFIG with (internal/plugin/manifests.CollectorPipelineConfig
// or MetricsPipelineConfig).
func New(ds *appsv1.DaemonSet, pipeline *flpconfig.Root) (*Builder, error) {
	if ds == nil {
		return nil, fmt.Errorf("daemonset is nil")
	}
	if len(ds.Spec.Template.Spec.Containers) == 0 {
		return nil, fmt.Errorf("daemonset %q has no containers", ds.Name)
	}
	if pipeline == nil {
		return nil, fmt.Errorf("pipeline for daemonset %q is nil", ds.Name)
	}
	return &Builder{ds: ds, pipeline: pipeline, filters: []agentconfig.FlowFilter{}}, nil
}

// Build finalizes the DaemonSet: marshals the pipeline and flow-filter rules back into their env
// vars and returns the completed object.
func (b *Builder) Build() (*appsv1.DaemonSet, error) {
	pipelineJSON, err := json.Marshal(b.pipeline)
	if err != nil {
		return nil, fmt.Errorf("cannot marshal FLP pipeline config: %w", err)
	}
	b.setEnv(envFLPConfig, string(pipelineJSON))

	filters := b.filters
	if filters == nil {
		filters = []agentconfig.FlowFilter{}
	}
	filtersJSON, err := json.Marshal(filters)
	if err != nil {
		return nil, fmt.Errorf("cannot marshal flow filter rules: %w", err)
	}
	b.setEnv(envFlowFilterRules, string(filtersJSON))

	return b.ds, nil
}

// SetFlowFilters replaces the complete eBPF filter list. The CLI parser owns
// command-line syntax; this method is the boundary where its typed result is
// attached to the agent specification.
func (b *Builder) SetFlowFilters(filters []agentconfig.FlowFilter) {
	b.filters = append([]agentconfig.FlowFilter(nil), filters...)
}

func (b *Builder) container() *corev1.Container {
	return &b.ds.Spec.Template.Spec.Containers[0]
}

// setEnv sets or appends an env var on the capture container. Every name this package sets is
// already present in the DaemonSet templates, so append is just a defensive fallback: unlike
// yq's `select(.name==X).value|=Y` (a silent no-op when X isn't found), this never silently
// drops a mutation.
func (b *Builder) setEnv(name, value string) {
	c := b.container()
	for i := range c.Env {
		if c.Env[i].Name == name {
			c.Env[i].Value = value
			return
		}
	}
	c.Env = append(c.Env, corev1.EnvVar{Name: name, Value: value})
}

// pipelineStage finds a FLP pipeline stage parameter by name (e.g. "enrich", "send",
// "prometheus"), replacing yq's `.parameters[] | select(.name==X) | path | .[-1]` index lookup.
func (b *Builder) pipelineStage(name string) (*flpconfig.StageParam, error) {
	for i := range b.pipeline.Parameters {
		if b.pipeline.Parameters[i].Name == name {
			return &b.pipeline.Parameters[i], nil
		}
	}
	return nil, fmt.Errorf("FLP pipeline has no %q stage", name)
}
