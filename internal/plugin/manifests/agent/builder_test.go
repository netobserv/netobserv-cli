package agent

import (
	"encoding/json"
	"testing"

	flpapi "github.com/netobserv/flowlogs-pipeline/pkg/api"
	flpconfig "github.com/netobserv/flowlogs-pipeline/pkg/config"
	agentconfig "github.com/netobserv/netobserv-ebpf-agent/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/netobserv/network-observability-cli/internal/plugin/manifests"
)

func newFlowsBuilder(t *testing.T) *Builder {
	t.Helper()
	ds, err := manifests.BuildDaemonSet(manifests.Flows, "netobserv-cli", "quay.io/netobserv/netobserv-ebpf-agent:main")
	require.NoError(t, err)
	pipeline, err := manifests.CollectorPipelineConfig("collector.netobserv-cli.svc.cluster.local")
	require.NoError(t, err)
	b, err := New(ds, pipeline)
	require.NoError(t, err)
	return b
}

func newMetricsBuilder(t *testing.T) *Builder {
	t.Helper()
	ds, err := manifests.BuildDaemonSet(manifests.Metrics, "netobserv-cli", "quay.io/netobserv/netobserv-ebpf-agent:main")
	require.NoError(t, err)
	pipeline, err := manifests.MetricsPipelineConfig()
	require.NoError(t, err)
	b, err := New(ds, pipeline)
	require.NoError(t, err)
	return b
}

func envValue(t *testing.T, ds *appsv1.DaemonSet, name string) string {
	t.Helper()
	for _, e := range ds.Spec.Template.Spec.Containers[0].Env {
		if e.Name == name {
			return e.Value
		}
	}
	t.Fatalf("env var %q not found", name)
	return ""
}

func TestNewRejectsContainerlessDaemonSet(t *testing.T) {
	_, err := New(&appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "empty"}}, &flpconfig.Root{})
	assert.Error(t, err)
}

func TestBuildDefaultsFLPConfigAndEmptyFilters(t *testing.T) {
	b := newFlowsBuilder(t)
	ds, err := b.Build()
	require.NoError(t, err)

	assert.JSONEq(t, "[]", envValue(t, ds, envFlowFilterRules))

	var root flpconfig.Root
	require.NoError(t, json.Unmarshal([]byte(envValue(t, ds, envFLPConfig)), &root))
	assert.NotEmpty(t, root.Parameters)
}

func TestSimpleEnvSetters(t *testing.T) {
	b := newFlowsBuilder(t)
	b.SetSampling(50)
	b.SetInterfaces([]string{"eth0", "eth1"})
	b.SetExcludeInterfaces([]string{"lo"})
	b.SetLogLevel("debug")
	b.SetNodeSelector("netobserv", "true")

	ds, err := b.Build()
	require.NoError(t, err)

	assert.Equal(t, "50", envValue(t, ds, envSampling))
	assert.Equal(t, "eth0,eth1", envValue(t, ds, envInterfaces))
	assert.Equal(t, "lo", envValue(t, ds, envExcludeInterfaces))
	assert.Equal(t, "debug", envValue(t, ds, envLogLevel))
	assert.Equal(t, map[string]string{"netobserv": "true"}, ds.Spec.Template.Spec.NodeSelector)
}

func TestEnablePacketDropsSetsPrivileged(t *testing.T) {
	b := newFlowsBuilder(t)
	b.EnablePacketDrops()
	ds, err := b.Build()
	require.NoError(t, err)

	assert.Equal(t, "true", envValue(t, ds, envPktDrops))
	sc := ds.Spec.Template.Spec.Containers[0].SecurityContext
	require.NotNil(t, sc)
	assert.True(t, *sc.Privileged)
	assert.True(t, *sc.AllowPrivilegeEscalation)
}

func TestEnableDNSTrackingDoesNotTouchPrivileged(t *testing.T) {
	b := newFlowsBuilder(t)
	b.EnableDNSTracking()
	ds, err := b.Build()
	require.NoError(t, err)

	assert.Equal(t, "true", envValue(t, ds, envDNSTracking))
	sc := ds.Spec.Template.Spec.Containers[0].SecurityContext
	require.NotNil(t, sc, "the template itself already sets a securityContext")
	assert.False(t, *sc.Privileged, "--enable_dns must not request privileged mode")
	assert.False(t, *sc.AllowPrivilegeEscalation)
}

func TestEnableAllSetsEveryFlowsFeatureButNotOpenSSL(t *testing.T) {
	b := newFlowsBuilder(t)
	require.NoError(t, b.EnableAll())
	ds, err := b.Build()
	require.NoError(t, err)

	for _, name := range []string{envPktDrops, envDNSTracking, envRTT, envNetworkEvents, envUDNMapping, envPktTranslation, envIPSecTracking} {
		assert.Equal(t, "true", envValue(t, ds, name), "env %s", name)
	}
	// flow-capture.yml (like bash's enable_all) has no OpenSSL-tracking env var at all: that
	// feature is packets-only and enable_all never touches it.
	assert.False(t, hasEnv(ds, envOpenSSLTracking), "enable_all must not touch OpenSSL tracking")
}

func TestEnableOpenSSLTrackingAddsHostMountsOnce(t *testing.T) {
	b := newFlowsBuilder(t)
	b.EnableOpenSSLTracking()
	b.AddHostMountsForTLS() // simulate the CLI calling it twice; must stay idempotent
	ds, err := b.Build()
	require.NoError(t, err)

	assert.Equal(t, "true", envValue(t, ds, envOpenSSLTracking))
	assert.True(t, ds.Spec.Template.Spec.HostPID)

	volumeNames := map[string]int{}
	for _, v := range ds.Spec.Template.Spec.Volumes {
		volumeNames[v.Name]++
	}
	assert.Equal(t, 1, volumeNames["host-usr"])
	assert.Equal(t, 1, volumeNames["host-lib"])
	assert.Equal(t, 1, volumeNames["host-lib64"])

	sc := ds.Spec.Template.Spec.Containers[0].SecurityContext
	require.NotNil(t, sc)
	assert.False(t, *sc.ReadOnlyRootFilesystem)
	ptraceCount := 0
	for _, c := range sc.Capabilities.Add {
		if c == "SYS_PTRACE" {
			ptraceCount++
		}
	}
	assert.Equal(t, 1, ptraceCount)
}

func TestAddHostMountsForTLSCompletesPartialConfiguration(t *testing.T) {
	b := newFlowsBuilder(t)
	spec := &b.ds.Spec.Template.Spec
	spec.Volumes = append(spec.Volumes, hostPathVolume("host-usr", "/usr"))
	b.container().VolumeMounts = append(b.container().VolumeMounts, corev1.VolumeMount{
		Name: "host-usr", MountPath: "/host/usr", ReadOnly: true,
	})

	b.AddHostMountsForTLS()
	b.AddHostMountsForTLS()

	for _, name := range []string{"host-usr", "host-lib", "host-lib64"} {
		assert.Equal(t, 1, countVolumes(spec.Volumes, name), "volume %s", name)
		assert.Equal(t, 1, countVolumeMounts(b.container().VolumeMounts, name), "mount %s", name)
	}
}

func countVolumes(volumes []corev1.Volume, name string) int {
	count := 0
	for i := range volumes {
		volume := &volumes[i]
		if volume.Name == name {
			count++
		}
	}
	return count
}

func countVolumeMounts(mounts []corev1.VolumeMount, name string) int {
	count := 0
	for _, mount := range mounts {
		if mount.Name == name {
			count++
		}
	}
	return count
}

func TestEnableCollectorTLS(t *testing.T) {
	b := newFlowsBuilder(t)
	require.NoError(t, b.EnableCollectorTLS("/etc/collector-ca/service-ca.crt"))
	ds, err := b.Build()
	require.NoError(t, err)

	c := ds.Spec.Template.Spec.Containers[0]
	assert.Contains(t, mountNames(c.VolumeMounts), "collector-ca")
	require.Len(t, c.EnvFrom, 1)
	assert.Equal(t, "collector-tls-config", c.EnvFrom[0].ConfigMapRef.Name)
	assert.False(t, *c.EnvFrom[0].ConfigMapRef.Optional)

	var root flpconfig.Root
	require.NoError(t, json.Unmarshal([]byte(envValue(t, ds, envFLPConfig)), &root))
	send := findStage(t, &root, "send")
	require.NotNil(t, send.Write.GRPC.TLS)
	assert.Equal(t, "/etc/collector-ca/service-ca.crt", send.Write.GRPC.TLS.CACertPath)
}

func TestEnableUDNMapping(t *testing.T) {
	b := newFlowsBuilder(t)
	require.NoError(t, b.EnableUDNMapping())
	ds, err := b.Build()
	require.NoError(t, err)

	assert.Equal(t, "true", envValue(t, ds, envUDNMapping))
	sc := ds.Spec.Template.Spec.Containers[0].SecurityContext
	require.NotNil(t, sc)
	assert.True(t, *sc.Privileged)

	var root flpconfig.Root
	require.NoError(t, json.Unmarshal([]byte(envValue(t, ds, envFLPConfig)), &root))
	enrich := findStage(t, &root, "enrich")
	require.Len(t, enrich.Transform.Network.KubeConfig.SecondaryNetworks, 1)
	assert.Equal(t, "ovn-kubernetes", enrich.Transform.Network.KubeConfig.SecondaryNetworks[0].Name)
}

func TestSetSubnetLabelsSortedByCategory(t *testing.T) {
	b := newFlowsBuilder(t)
	require.NoError(t, b.SetSubnetLabels(map[string][]string{
		"Services": {"172.30.0.0/16"},
		"Machines": {"10.0.0.0/16"},
		"Pods":     {"10.128.0.0/14", "fd01::/48"},
	}))
	ds, err := b.Build()
	require.NoError(t, err)

	var root flpconfig.Root
	require.NoError(t, json.Unmarshal([]byte(envValue(t, ds, envFLPConfig)), &root))
	enrich := findStage(t, &root, "enrich")

	require.Len(t, enrich.Transform.Network.SubnetLabels, 3)
	assert.Equal(t, "Machines", enrich.Transform.Network.SubnetLabels[0].Name)
	assert.Equal(t, "Pods", enrich.Transform.Network.SubnetLabels[1].Name)
	assert.Equal(t, []string{"10.128.0.0/14", "fd01::/48"}, enrich.Transform.Network.SubnetLabels[1].CIDRs)
	assert.Equal(t, "Services", enrich.Transform.Network.SubnetLabels[2].Name)
}

func TestFlowFilterFirstFieldStartsAFilter(t *testing.T) {
	b := newFlowsBuilder(t)
	b.SetFlowFilters([]agentconfig.FlowFilter{{IPCIDR: "0.0.0.0/0", Action: "Accept", Protocol: "TCP", Port: 8080}})
	ds, err := b.Build()
	require.NoError(t, err)

	var filters []agentconfig.FlowFilter
	require.NoError(t, json.Unmarshal([]byte(envValue(t, ds, envFlowFilterRules)), &filters))
	require.Len(t, filters, 1)
	assert.Equal(t, "TCP", filters[0].Protocol)
	assert.Equal(t, int32(8080), filters[0].Port)
	assert.Equal(t, "Accept", filters[0].Action, "default rule's Action must survive as the base for the first filter")
}

func TestFlowFilterOrStartsANewGroup(t *testing.T) {
	b := newFlowsBuilder(t)
	b.SetFlowFilters([]agentconfig.FlowFilter{
		{IPCIDR: "0.0.0.0/0", Action: "Accept", Protocol: "TCP", Port: 8080},
		{IPCIDR: "0.0.0.0/0", Action: "Accept", Protocol: "UDP"},
	})

	ds, err := b.Build()
	require.NoError(t, err)

	var filters []agentconfig.FlowFilter
	require.NoError(t, json.Unmarshal([]byte(envValue(t, ds, envFlowFilterRules)), &filters))
	require.Len(t, filters, 2)
	assert.Equal(t, "TCP", filters[0].Protocol)
	assert.Equal(t, int32(8080), filters[0].Port)
	assert.Equal(t, "UDP", filters[1].Protocol)
	assert.Equal(t, int32(0), filters[1].Port, "the second filter must be a fresh default, not sharing state with the first")
}

func TestSetFilterQueryInsertsThenExtendsFilterStage(t *testing.T) {
	b := newFlowsBuilder(t)
	b.AddFilterQuery(`SrcPort>1024`)
	b.AddFilterQuery(`DstPort<80`)

	ds, err := b.Build()
	require.NoError(t, err)

	var root flpconfig.Root
	require.NoError(t, json.Unmarshal([]byte(envValue(t, ds, envFLPConfig)), &root))

	// pipeline must be enrich -> filter -> send, in that order.
	names := make([]string, len(root.Pipeline))
	for i, s := range root.Pipeline {
		names[i] = s.Name
	}
	assert.Equal(t, []string{"enrich", "filter", "send"}, names)

	filterStage := findStage(t, &root, "filter")
	require.Len(t, filterStage.Transform.Filter.Rules, 2)
	assert.Equal(t, flpapi.KeepEntryQuery, filterStage.Transform.Filter.Rules[0].Type)
	assert.Equal(t, "SrcPort>1024", filterStage.Transform.Filter.Rules[0].KeepEntryQuery)
	assert.Equal(t, "DstPort<80", filterStage.Transform.Filter.Rules[1].KeepEntryQuery)
}

func TestRestrictMetrics(t *testing.T) {
	b := newMetricsBuilder(t)
	matched, err := b.RestrictMetrics([]string{"namespace_flows_total", "node_ingress_bytes_total"})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"namespace_flows_total", "node_ingress_bytes_total"}, matched)

	ds, err := b.Build()
	require.NoError(t, err)

	var root flpconfig.Root
	require.NoError(t, json.Unmarshal([]byte(envValue(t, ds, envFLPConfig)), &root))
	prom := findStage(t, &root, "prometheus")

	names := make([]string, len(prom.Encode.Prom.Metrics))
	for i, m := range prom.Encode.Prom.Metrics {
		names[i] = m.Name
	}
	assert.ElementsMatch(t, []string{"namespace_flows_total", "node_ingress_bytes_total"}, names)
}

func hasEnv(ds *appsv1.DaemonSet, name string) bool {
	for _, e := range ds.Spec.Template.Spec.Containers[0].Env {
		if e.Name == name {
			return true
		}
	}
	return false
}

func mountNames(mounts []corev1.VolumeMount) []string {
	names := make([]string, len(mounts))
	for i, m := range mounts {
		names[i] = m.Name
	}
	return names
}

func findStage(t *testing.T, root *flpconfig.Root, name string) *flpconfig.StageParam {
	t.Helper()
	for i := range root.Parameters {
		if root.Parameters[i].Name == name {
			return &root.Parameters[i]
		}
	}
	t.Fatalf("stage %q not found", name)
	return nil
}
