package manifests

import (
	"encoding/json"
	"fmt"

	flpconfig "github.com/netobserv/flowlogs-pipeline/pkg/config"
)

// CollectorPipelineConfig decodes res/collector-pipeline-config.json (the FlowLogs-Pipeline
// config used by flows and packets captures) into the typed Root the flowlogs-pipeline module
// already defines for its own FLP_CONFIG, substituting the collector Service's target host.
// Replaces setCollectorPipelineConfig's yq-based JSON injection.
func CollectorPipelineConfig(targetHost string) (*flpconfig.Root, error) {
	data, err := readTemplate("collector-pipeline-config.json", map[string]string{"{{TARGET_HOST}}": targetHost})
	if err != nil {
		return nil, err
	}
	root := &flpconfig.Root{}
	if err := json.Unmarshal(data, root); err != nil {
		return nil, fmt.Errorf("cannot decode collector pipeline config: %w", err)
	}
	return root, nil
}

// MetricsPipelineConfig decodes res/metrics-pipeline-config.json (the FLP config used by
// metrics captures) into the typed Root. Replaces setMetricsPipelineConfig.
func MetricsPipelineConfig() (*flpconfig.Root, error) {
	data, err := readTemplate("metrics-pipeline-config.json", nil)
	if err != nil {
		return nil, err
	}
	root := &flpconfig.Root{}
	if err := json.Unmarshal(data, root); err != nil {
		return nil, fmt.Errorf("cannot decode metrics pipeline config: %w", err)
	}
	return root, nil
}
