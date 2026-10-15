package manifests

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// BuildRBACBundle decodes res/service-account.yml: the ServiceAccount, ClusterRole,
// ClusterRoleBindings, SecurityContextConstraints, Role and RoleBinding the DaemonSet/collector
// run as. The SCC is OpenShift-only and is omitted on Kubernetes clusters.
func BuildRBACBundle(namespace string, openShift bool) ([]runtime.Object, error) {
	data, err := readTemplate("service-account.yml", map[string]string{"{{NAMESPACE}}": namespace})
	if err != nil {
		return nil, err
	}
	objects, err := decodeAllUnstructured(data)
	if err != nil || openShift {
		return objects, err
	}

	filtered := make([]runtime.Object, 0, len(objects))
	for _, object := range objects {
		u := object.(*unstructured.Unstructured)
		if u.GetKind() != "SecurityContextConstraints" {
			filtered = append(filtered, object)
		}
	}
	return filtered, nil
}

// BuildMetricsBundle decodes res/service-monitor.yml: the ClusterRole/ClusterRoleBinding
// letting openshift-monitoring scrape the collector, and the ServiceMonitor itself. Applied
// as-is, never mutated at runtime.
func BuildMetricsBundle(namespace string) ([]runtime.Object, error) {
	data, err := readTemplate("service-monitor.yml", map[string]string{"{{NAMESPACE}}": namespace})
	if err != nil {
		return nil, err
	}
	return decodeAllUnstructured(data)
}
