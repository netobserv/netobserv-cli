package commands

import (
	"encoding/json"
	"testing"

	agentconfig "github.com/netobserv/netobserv-ebpf-agent/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netobserv/network-observability-cli/internal/plugin/manifests"
	"github.com/netobserv/network-observability-cli/internal/plugin/manifests/agent"
)

func TestSplitFilterArgs(t *testing.T) {
	groups, queries, rest := splitFilterArgs([]string{
		"--protocol=TCP", "--port=8080", "or", "--protocol=UDP",
		"--query=SrcPort>1024", "--log-level=debug",
	})

	require.Len(t, groups, 2)
	assert.Equal(t, []filterToken{{key: "protocol", value: "TCP"}, {key: "port", value: "8080"}}, groups[0])
	assert.Equal(t, []filterToken{{key: "protocol", value: "UDP"}}, groups[1])
	assert.Equal(t, []string{"SrcPort>1024"}, queries)
	assert.Equal(t, []string{"--log-level=debug"}, rest)
}

func TestArgsAfterCommand(t *testing.T) {
	assert.Equal(t, []string{"--protocol=TCP", "or", "--protocol=UDP"}, argsAfterCommand(
		[]string{"--namespace", "capture", "flows", "--protocol=TCP", "or", "--protocol=UDP"}, "flows"))
	assert.Nil(t, argsAfterCommand([]string{"version"}, "flows"))
}

func TestSplitFilterArgsBareDrops(t *testing.T) {
	groups, _, rest := splitFilterArgs([]string{"--drops", "--background"})
	require.Len(t, groups, 1)
	assert.Equal(t, []filterToken{{key: "drops", value: "true"}}, groups[0])
	assert.Equal(t, []string{"--background"}, rest)
}

func newTestBuilder(t *testing.T) *agent.Builder {
	t.Helper()
	ds, err := manifests.BuildDaemonSet(manifests.Flows, "netobserv-cli", "img")
	require.NoError(t, err)
	pipeline, err := manifests.CollectorPipelineConfig("collector.netobserv-cli.svc.cluster.local")
	require.NoError(t, err)
	b, err := agent.New(ds, pipeline)
	require.NoError(t, err)
	return b
}

func flowFiltersOf(t *testing.T, b *agent.Builder) []agentconfig.FlowFilter {
	t.Helper()
	ds, err := b.Build()
	require.NoError(t, err)
	var filters []agentconfig.FlowFilter
	for _, e := range ds.Spec.Template.Spec.Containers[0].Env {
		if e.Name == "FLOW_FILTER_RULES" {
			require.NoError(t, json.Unmarshal([]byte(e.Value), &filters))
		}
	}
	return filters
}

func envOf(t *testing.T, b *agent.Builder, name string) string {
	t.Helper()
	ds, err := b.Build()
	require.NoError(t, err)
	for _, e := range ds.Spec.Template.Spec.Containers[0].Env {
		if e.Name == name {
			return e.Value
		}
	}
	t.Fatalf("env %s not found", name)
	return ""
}

func TestApplyFilterGroupsTwoGroups(t *testing.T) {
	b := newTestBuilder(t)
	groups, _, _ := splitFilterArgs([]string{"--protocol=TCP", "--port=8080", "or", "--protocol=UDP"})
	require.NoError(t, applyFilterGroups(b, groups))

	filters := flowFiltersOf(t, b)
	require.Len(t, filters, 2)
	assert.Equal(t, "TCP", filters[0].Protocol)
	assert.Equal(t, int32(8080), filters[0].Port)
	assert.Equal(t, "UDP", filters[1].Protocol)
	assert.Equal(t, int32(0), filters[1].Port)
}

func TestApplyFilterGroupsInvalidProtocol(t *testing.T) {
	b := newTestBuilder(t)
	groups, _, _ := splitFilterArgs([]string{"--protocol=BOGUS"})
	assert.Error(t, applyFilterGroups(b, groups))
}

func TestApplyFilterGroupsDropsEnablesPrivilegedAndPktDrops(t *testing.T) {
	b := newTestBuilder(t)
	groups, _, _ := splitFilterArgs([]string{"--drops"})
	require.NoError(t, applyFilterGroups(b, groups))

	filters := flowFiltersOf(t, b)
	require.Len(t, filters, 1)
	assert.True(t, filters[0].Drops)
	assert.Equal(t, "true", envOf(t, b, "ENABLE_PKT_DROPS"))
}

func TestApplyFilterGroupsNoFiltersLeavesEmptyArray(t *testing.T) {
	b := newTestBuilder(t)
	groups, _, _ := splitFilterArgs([]string{"--log-level=debug"})
	require.NoError(t, applyFilterGroups(b, groups))
	assert.Empty(t, flowFiltersOf(t, b))
}
