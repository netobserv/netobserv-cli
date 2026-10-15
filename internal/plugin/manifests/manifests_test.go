package manifests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestBuildNamespace(t *testing.T) {
	ns := BuildNamespace("netobserv-cli")
	assert.Equal(t, "Namespace", ns.Kind)
	assert.Equal(t, "v1", ns.APIVersion)
	assert.Equal(t, "netobserv-cli", ns.Name)
	assert.Equal(t, "netobserv-cli", ns.Labels["app"])
	assert.Equal(t, "privileged", ns.Labels["pod-security.kubernetes.io/enforce"])
}

func TestBuildCollectorService(t *testing.T) {
	svc := BuildCollectorService("my-ns")
	assert.Equal(t, "Service", svc.Kind)
	assert.Equal(t, "my-ns", svc.Namespace)
	assert.Equal(t, "collector", svc.Name)
	require.Len(t, svc.Spec.Ports, 1)
	assert.Equal(t, int32(9999), svc.Spec.Ports[0].Port)
}

func TestBuildCAConfigMap(t *testing.T) {
	cm := BuildCAConfigMap("my-ns")
	assert.Equal(t, "collector-ca", cm.Name)
	assert.Equal(t, "my-ns", cm.Namespace)
	assert.Equal(t, "true", cm.Annotations["service.beta.openshift.io/inject-cabundle"])
}

func TestBuildRBACBundle(t *testing.T) {
	objs, err := BuildRBACBundle("my-ns", true)
	require.NoError(t, err)
	// ServiceAccount, ClusterRole, 2x ClusterRoleBinding, SecurityContextConstraints, Role, RoleBinding.
	require.Len(t, objs, 7)
	assert.Equal(t, []string{
		"ServiceAccount", "ClusterRole", "ClusterRoleBinding", "ClusterRoleBinding",
		"SecurityContextConstraints", "Role", "RoleBinding",
	}, kindsOf(t, objs))
}

func TestBuildRBACBundleForKubernetesOmitsOpenShiftSCC(t *testing.T) {
	objs, err := BuildRBACBundle("my-ns", false)
	require.NoError(t, err)
	assert.Equal(t, []string{
		"ServiceAccount", "ClusterRole", "ClusterRoleBinding", "ClusterRoleBinding",
		"Role", "RoleBinding",
	}, kindsOf(t, objs))
}

func TestBuildMetricsBundle(t *testing.T) {
	objs, err := BuildMetricsBundle("my-ns")
	require.NoError(t, err)
	// ClusterRole, ClusterRoleBinding (scrape RBAC), ServiceMonitor, Service (metrics port),
	// ConfigMap (OCP console dashboard, lives in openshift-config-managed).
	require.Len(t, objs, 5)
	assert.Equal(t, []string{
		"ClusterRole", "ClusterRoleBinding", "ServiceMonitor", "Service", "ConfigMap",
	}, kindsOf(t, objs))
}

func TestBuildDaemonSet(t *testing.T) {
	for _, kind := range []CaptureKind{Flows, Packets, Metrics} {
		ds, err := BuildDaemonSet(kind, "my-ns", "quay.io/netobserv/netobserv-ebpf-agent:main")
		require.NoError(t, err, "kind=%s", kind)
		assert.Equal(t, "DaemonSet", ds.Kind)
		assert.Equal(t, "my-ns", ds.Namespace)
		require.Len(t, ds.Spec.Template.Spec.Containers, 1)
		assert.Equal(t, "quay.io/netobserv/netobserv-ebpf-agent:main", ds.Spec.Template.Spec.Containers[0].Image)
	}
}

func TestBuildDaemonSetUnknownKind(t *testing.T) {
	_, err := BuildDaemonSet(CaptureKind("bogus"), "my-ns", "img")
	assert.Error(t, err)
}

func TestCollectorPipelineConfigSubstitutesTargetHost(t *testing.T) {
	root, err := CollectorPipelineConfig("collector.my-ns.svc.cluster.local")
	require.NoError(t, err)

	found := false
	for _, p := range root.Parameters {
		if p.Name == "send" {
			require.NotNil(t, p.Write)
			require.NotNil(t, p.Write.GRPC)
			assert.Equal(t, "collector.my-ns.svc.cluster.local", p.Write.GRPC.TargetHost)
			assert.Equal(t, 9999, p.Write.GRPC.TargetPort)
			found = true
		}
	}
	assert.True(t, found, "expected a 'send' stage parameter")
}

func TestMetricsPipelineConfigHasExpectedStages(t *testing.T) {
	root, err := MetricsPipelineConfig()
	require.NoError(t, err)

	foundProm := false
	for _, p := range root.Parameters {
		if p.Name == "prometheus" {
			require.NotNil(t, p.Encode)
			require.NotNil(t, p.Encode.Prom)
			assert.NotEmpty(t, p.Encode.Prom.Metrics)
			foundProm = true
		}
	}
	assert.True(t, foundProm, "expected a 'prometheus' stage parameter")
}

func kindsOf(t *testing.T, objs []runtime.Object) []string {
	t.Helper()
	kinds := make([]string, len(objs))
	for i, obj := range objs {
		u, ok := obj.(*unstructured.Unstructured)
		require.True(t, ok, "object %T is not unstructured", obj)
		kinds[i] = u.GetKind()
	}
	return kinds
}
