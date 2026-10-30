package commands

import (
	"fmt"

	"github.com/netobserv/netobserv-operator/pkg/capture"
	"github.com/netobserv/network-observability-cli/internal/plugin/manifests/agent"
)

// newCaptureBuilder is the one place where a capture kind is turned into its
// manifest and pipeline model. Command handlers should only describe the
// options unique to their command; deployment assembly belongs here.
func newCaptureBuilder(kind capture.Type, options *globalOptions) (*agent.Builder, error) {
	pipeline := capture.MetricsPipelineConfig()
	if kind != capture.Metrics {
		pipeline = capture.CollectorPipelineConfig(fmt.Sprintf("collector.%s.svc.cluster.local", options.namespace))
	}
	ds := capture.BuildAgentDaemonSet(capture.AgentOptions{
		Name: "netobserv-cli", Namespace: options.namespace, ServiceAccount: "netobserv-cli", Image: options.agentImage,
		Type: kind, Sampling: 1, ExcludeInterfaces: []string{"lo"}, Labels: map[string]string{"app": "netobserv-cli"}, ContainerName: "netobserv-cli",
	})
	return agent.New(ds, pipeline)
}
