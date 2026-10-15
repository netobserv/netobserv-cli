package commands

import (
	"fmt"

	flpconfig "github.com/netobserv/flowlogs-pipeline/pkg/config"

	"github.com/netobserv/network-observability-cli/internal/plugin/manifests"
	"github.com/netobserv/network-observability-cli/internal/plugin/manifests/agent"
)

// newCaptureBuilder is the one place where a capture kind is turned into its
// manifest and pipeline model. Command handlers should only describe the
// options unique to their command; deployment assembly belongs here.
func newCaptureBuilder(kind manifests.CaptureKind, options *globalOptions) (*agent.Builder, error) {
	var (
		pipeline *flpconfig.Root
		err      error
	)
	if kind == manifests.Metrics {
		pipeline, err = manifests.MetricsPipelineConfig()
	} else {
		pipeline, err = manifests.CollectorPipelineConfig(fmt.Sprintf("collector.%s.svc.cluster.local", options.namespace))
	}
	if err != nil {
		return nil, err
	}

	ds, err := manifests.BuildDaemonSet(kind, options.namespace, options.agentImage)
	if err != nil {
		return nil, err
	}
	return agent.New(ds, pipeline)
}
