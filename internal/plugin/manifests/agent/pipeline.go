package agent

import (
	"fmt"
	"strings"

	flpapi "github.com/netobserv/flowlogs-pipeline/pkg/api"
	flpconfig "github.com/netobserv/flowlogs-pipeline/pkg/config"
)

// AddFilterQuery adds a FlowLogs-Pipeline keep-entry query between enrichment
// and export. It is independent from the eBPF agent's FlowFilter list.
func (b *Builder) AddFilterQuery(query string) {
	rule := flpapi.TransformFilterRule{Type: flpapi.KeepEntryQuery, KeepEntryQuery: query}

	for i := range b.pipeline.Parameters {
		if b.pipeline.Parameters[i].Name == "filter" {
			b.pipeline.Parameters[i].Transform.Filter.Rules = append(b.pipeline.Parameters[i].Transform.Filter.Rules, rule)
			return
		}
	}

	b.pipeline.Parameters = append(b.pipeline.Parameters, flpconfig.StageParam{
		Name: "filter",
		Transform: &flpconfig.Transform{
			Type:   "filter",
			Filter: &flpapi.TransformFilter{Rules: []flpapi.TransformFilterRule{rule}},
		},
	})

	// Keep the stage ordering explicit instead of removing and re-adding
	// arbitrary pipeline entries as the old shell implementation did.
	newPipeline := make([]flpconfig.Stage, 0, len(b.pipeline.Pipeline)+1)
	for i := range b.pipeline.Pipeline {
		if b.pipeline.Pipeline[i].Name != "send" {
			newPipeline = append(newPipeline, b.pipeline.Pipeline[i])
		}
	}
	newPipeline = append(newPipeline,
		flpconfig.Stage{Name: "filter", Follows: "enrich"},
		flpconfig.Stage{Name: "send", Follows: "filter"},
	)
	b.pipeline.Pipeline = newPipeline
}

// RestrictMetrics keeps metrics whose names contain one of the requested
// substrings and returns the names that were retained in request order.
func (b *Builder) RestrictMetrics(substrings []string) ([]string, error) {
	prom, err := b.pipelineStage("prometheus")
	if err != nil {
		return nil, err
	}
	if prom.Encode == nil || prom.Encode.Prom == nil {
		return nil, fmt.Errorf("FLP 'prometheus' stage has no prom encode configuration")
	}

	requested := make(map[string]bool)
	var matched []string
	for _, substring := range substrings {
		for i := range prom.Encode.Prom.Metrics {
			metric := &prom.Encode.Prom.Metrics[i]
			if strings.Contains(metric.Name, substring) && !requested[metric.Name] {
				requested[metric.Name] = true
				matched = append(matched, metric.Name)
			}
		}
	}

	kept := make([]flpapi.MetricsItem, 0, len(matched))
	for i := range prom.Encode.Prom.Metrics {
		metric := prom.Encode.Prom.Metrics[i]
		if requested[metric.Name] {
			kept = append(kept, metric)
		}
	}
	prom.Encode.Prom.Metrics = kept
	return matched, nil
}
