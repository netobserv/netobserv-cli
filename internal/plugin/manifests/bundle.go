package manifests

import (
	"github.com/netobserv/netobserv-operator/pkg/capture"
	securityv1 "github.com/openshift/api/security/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// BuildRBACBundle builds the ServiceAccount, RBAC and optional OpenShift SCC
// required by the DaemonSet and collector.
func BuildRBACBundle(namespace string, openShift bool) ([]runtime.Object, error) {
	name := "netobserv-cli"
	objects := []runtime.Object{
		capture.BuildServiceAccount(name, namespace),
		&rbacv1.ClusterRole{
			TypeMeta:   metav1.TypeMeta{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "ClusterRole"},
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Rules: []rbacv1.PolicyRule{
				{APIGroups: []string{"apps"}, Resources: []string{"daemonsets"}, Verbs: []string{"get", "list", "watch", "delete"}},
				{APIGroups: []string{""}, Resources: []string{"pods", "services", "nodes"}, Verbs: []string{"get", "list", "watch"}},
				{APIGroups: []string{"apps"}, Resources: []string{"replicasets"}, Verbs: []string{"get", "list", "watch"}},
				{APIGroups: []string{"config.openshift.io"}, Resources: []string{"apiservers"}, ResourceNames: []string{"cluster"}, Verbs: []string{"get"}},
			},
		},
		&rbacv1.ClusterRoleBinding{
			TypeMeta:   metav1.TypeMeta{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "ClusterRoleBinding"},
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: name, Namespace: namespace}},
			RoleRef:    rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "ClusterRole", Name: name},
		},
		&rbacv1.ClusterRoleBinding{
			TypeMeta:   metav1.TypeMeta{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "ClusterRoleBinding"},
			ObjectMeta: metav1.ObjectMeta{Name: name + "-monitoring"},
			Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: name, Namespace: namespace}},
			RoleRef:    rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "ClusterRole", Name: "cluster-monitoring-view"},
		},
		&rbacv1.Role{
			TypeMeta:   metav1.TypeMeta{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "Role"},
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Rules: []rbacv1.PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"pods", "namespaces", "nodes", "services", "endpoints"}, Verbs: []string{"get", "list", "watch", "patch"}},
				// The resolve-tls init container creates or updates this ConfigMap,
				// and the collector reads it through the Kubernetes API when it
				// starts. Keep this permission namespaced and limited to this object.
				{APIGroups: []string{""}, Resources: []string{"configmaps"}, Verbs: []string{"create"}},
				{APIGroups: []string{""}, Resources: []string{"configmaps"}, ResourceNames: []string{"collector-tls-config"}, Verbs: []string{"get", "update"}},
			},
		},
		&rbacv1.RoleBinding{
			TypeMeta:   metav1.TypeMeta{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "RoleBinding"},
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			RoleRef:    rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "Role", Name: name},
			Subjects:   []rbacv1.Subject{{Name: name, Namespace: namespace, Kind: "ServiceAccount"}},
		},
	}
	if openShift {
		scc := &securityv1.SecurityContextConstraints{
			TypeMeta:                 metav1.TypeMeta{APIVersion: "security.openshift.io/v1", Kind: "SecurityContextConstraints"},
			ObjectMeta:               metav1.ObjectMeta{Name: "netobserv-cli"},
			AllowHostDirVolumePlugin: true,
			AllowHostNetwork:         true,
			AllowPrivilegedContainer: true,
			RunAsUser:                securityv1.RunAsUserStrategyOptions{Type: securityv1.RunAsUserStrategyRunAsAny},
			SELinuxContext:           securityv1.SELinuxContextStrategyOptions{Type: securityv1.SELinuxStrategyRunAsAny},
			Users:                    []string{"system:serviceaccount:" + namespace + ":netobserv-cli"},
			AllowedCapabilities:      []corev1.Capability{"BPF", "PERFMON", "NET_ADMIN"},
		}
		objects = append(objects[:4], append([]runtime.Object{scc}, objects[4:]...)...)
	}
	return objects, nil
}

// BuildMetricsBundle combines the shared typed monitoring resources with the
// CLI-only dashboard ConfigMap. The monitoring contract itself lives in the
// operator capture package so it is identical for CLI and FlowCapture.
func BuildMetricsBundle(namespace string) ([]runtime.Object, error) {
	return append(capture.BuildMetricsResources(namespace), capture.BuildMetricsDashboard()), nil
}
