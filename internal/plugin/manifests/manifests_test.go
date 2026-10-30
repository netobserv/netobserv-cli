package manifests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
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

	role := objs[5].(*rbacv1.Role)
	assert.Contains(t, role.Rules, rbacv1.PolicyRule{APIGroups: []string{""}, Resources: []string{"configmaps"}, Verbs: []string{"create"}})
	assert.Contains(t, role.Rules, rbacv1.PolicyRule{APIGroups: []string{""}, Resources: []string{"configmaps"}, ResourceNames: []string{"collector-tls-config"}, Verbs: []string{"get", "update"}})
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

func kindsOf(t *testing.T, objs []runtime.Object) []string {
	t.Helper()
	kinds := make([]string, len(objs))
	for i, obj := range objs {
		kinds[i] = obj.GetObjectKind().GroupVersionKind().Kind
		require.NotEmpty(t, kinds[i], "object %T has no TypeMeta", obj)
	}
	return kinds
}
