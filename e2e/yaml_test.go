//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
)

// Decode YAML rather than depending on document order, quote style, or JSON
// key order. Set NETOBSERV_E2E_BINARY to run these tests without Kind:
// go test -tags=e2e e2e/yaml_test.go
func generatedResources(t *testing.T, mode string, args ...string) map[string]*unstructured.Unstructured {
	t.Helper()
	binary := os.Getenv("NETOBSERV_E2E_BINARY")
	if binary == "" {
		binary = "commands/oc-netobserv"
	}
	binary, err := filepath.Abs(binary)
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	args = append([]string{mode}, args...)
	args = append(args, "--yaml", "--namespace=netobserv-cli", "--output-dir="+dir)
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = append(os.Environ(), "isE2E=true")
	output, err := cmd.CombinedOutput()
	if !assert.NoError(t, err, string(output)) {
		t.FailNow()
	}
	if !assert.Contains(t, string(output), "Check the generated YAML file:") {
		t.FailNow()
	}
	files, err := filepath.Glob(filepath.Join(dir, mode+"_capture_*.yml"))
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	if !assert.Len(t, files, 1) {
		t.FailNow()
	}
	data, err := os.ReadFile(files[0])
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	decoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)
	resources := map[string]*unstructured.Unstructured{}
	for {
		obj := &unstructured.Unstructured{}
		err := decoder.Decode(obj)
		if errors.Is(err, io.EOF) {
			break
		}
		if !assert.NoError(t, err) {
			t.FailNow()
		}
		if len(obj.Object) == 0 {
			continue
		}
		key := obj.GetKind() + "/" + obj.GetName()
		if !assert.NotContains(t, resources, key, "duplicate YAML object") {
			t.FailNow()
		}
		resources[key] = obj
	}
	return resources
}

func resource(t *testing.T, objects map[string]*unstructured.Unstructured, kind, name string) *unstructured.Unstructured {
	t.Helper()
	obj := objects[kind+"/"+name]
	if !assert.NotNil(t, obj, "%s/%s missing", kind, name) {
		t.FailNow()
	}
	return obj
}

func checkBaseResources(t *testing.T, objects map[string]*unstructured.Unstructured) {
	t.Helper()
	ns := resource(t, objects, "Namespace", "netobserv-cli")
	assert.Equal(t, "netobserv-cli", ns.GetLabels()["app"])
	for _, label := range []string{"pod-security.kubernetes.io/enforce", "pod-security.kubernetes.io/audit"} {
		assert.Equal(t, "privileged", ns.GetLabels()[label])
	}
	assert.Equal(t, "true", ns.GetLabels()["openshift.io/cluster-monitoring"])
	assert.Equal(t, "netobserv-cli", resource(t, objects, "ServiceAccount", "netobserv-cli").GetNamespace())
	for _, obj := range objects {
		switch obj.GetKind() {
		case "Namespace", "ClusterRole", "ClusterRoleBinding", "SecurityContextConstraints":
			assert.Empty(t, obj.GetNamespace(), "%s is cluster scoped", obj.GetKind())
		default:
			if obj.GetKind() != "ConfigMap" {
				assert.Equal(t, "netobserv-cli", obj.GetNamespace())
			}
		}
	}
	role := &rbacv1.ClusterRole{}
	if !assert.NoError(t, runtime.DefaultUnstructuredConverter.FromUnstructured(resource(t, objects, "ClusterRole", "netobserv-cli").Object, role)) {
		t.FailNow()
	}
	for _, rule := range []rbacv1.PolicyRule{
		{APIGroups: []string{"security.openshift.io"}, Resources: []string{"securitycontextconstraints"}, ResourceNames: []string{"privileged"}, Verbs: []string{"use"}},
		{APIGroups: []string{"apps"}, Resources: []string{"daemonsets"}, Verbs: []string{"list", "get", "watch", "delete"}},
		{APIGroups: []string{""}, Resources: []string{"pods", "services", "nodes"}, Verbs: []string{"list", "get", "watch"}},
		{APIGroups: []string{"apps"}, Resources: []string{"replicasets"}, Verbs: []string{"list", "get", "watch"}},
		{APIGroups: []string{"config.openshift.io"}, Resources: []string{"apiservers"}, ResourceNames: []string{"cluster"}, Verbs: []string{"get"}},
	} {
		assert.Contains(t, role.Rules, rule)
	}
	for name, roleName := range map[string]string{"netobserv-cli": "netobserv-cli", "netobserv-cli-monitoring": "cluster-monitoring-view"} {
		binding := &rbacv1.ClusterRoleBinding{}
		if !assert.NoError(t, runtime.DefaultUnstructuredConverter.FromUnstructured(resource(t, objects, "ClusterRoleBinding", name).Object, binding)) {
			t.FailNow()
		}
		assert.Equal(t, rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "ClusterRole", Name: roleName}, binding.RoleRef)
		assert.Equal(t, []rbacv1.Subject{{Kind: "ServiceAccount", Name: "netobserv-cli", Namespace: "netobserv-cli"}}, binding.Subjects)
	}
	resource(t, objects, "SecurityContextConstraints", "netobserv-cli")
	localRole := &rbacv1.Role{}
	if !assert.NoError(t, runtime.DefaultUnstructuredConverter.FromUnstructured(resource(t, objects, "Role", "netobserv-cli").Object, localRole)) {
		t.FailNow()
	}
	assert.Contains(t, localRole.Rules, rbacv1.PolicyRule{APIGroups: []string{""}, Resources: []string{"configmaps"}, Verbs: []string{"get", "create", "update"}})
	binding := &rbacv1.RoleBinding{}
	if !assert.NoError(t, runtime.DefaultUnstructuredConverter.FromUnstructured(resource(t, objects, "RoleBinding", "netobserv-cli").Object, binding)) {
		t.FailNow()
	}
	assert.Equal(t, rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "Role", Name: "netobserv-cli"}, binding.RoleRef)
	assert.Equal(t, []rbacv1.Subject{{Kind: "ServiceAccount", Name: "netobserv-cli", Namespace: "netobserv-cli"}}, binding.Subjects)
}

func agent(t *testing.T, objects map[string]*unstructured.Unstructured) *appsv1.DaemonSet {
	t.Helper()
	ds := &appsv1.DaemonSet{}
	if !assert.NoError(t, runtime.DefaultUnstructuredConverter.FromUnstructured(resource(t, objects, "DaemonSet", "netobserv-cli").Object, ds)) {
		t.FailNow()
	}
	if !assert.Len(t, ds.Spec.Template.Spec.Containers, 1) {
		t.FailNow()
	}
	return ds
}
func envValue(t *testing.T, ds *appsv1.DaemonSet, name string) string {
	t.Helper()
	for _, value := range ds.Spec.Template.Spec.Containers[0].Env {
		if value.Name == name {
			return value.Value
		}
	}
	t.Fatalf("missing environment variable %s", name)
	return ""
}
func checkCollectorService(t *testing.T, objects map[string]*unstructured.Unstructured) {
	t.Helper()
	svc := &corev1.Service{}
	if !assert.NoError(t, runtime.DefaultUnstructuredConverter.FromUnstructured(resource(t, objects, "Service", "collector").Object, svc)) {
		t.FailNow()
	}
	if !assert.Len(t, svc.Spec.Ports, 1) {
		t.FailNow()
	}
	assert.Equal(t, int32(9999), svc.Spec.Ports[0].Port)
	assert.Equal(t, int32(9999), svc.Spec.Ports[0].TargetPort.IntVal)
	assert.Equal(t, corev1.ProtocolTCP, svc.Spec.Ports[0].Protocol)
}
func TestFlowFiltersYAML(t *testing.T) {
	objects := generatedResources(t, "flows", "--protocol=TCP", "--port=8080", "or", "--protocol=UDP")
	if !assert.Len(t, objects, 10) {
		t.FailNow()
	}
	checkBaseResources(t, objects)
	checkCollectorService(t, objects)
	ds := agent(t, objects)
	var filters []map[string]any
	if !assert.NoError(t, json.Unmarshal([]byte(envValue(t, ds, "FLOW_FILTER_RULES")), &filters)) {
		t.FailNow()
	}
	if !assert.Len(t, filters, 2) {
		t.FailNow()
	}
	assert.Equal(t, "TCP", filters[0]["protocol"])
	assert.Equal(t, float64(8080), filters[0]["port"])
	assert.Equal(t, "UDP", filters[1]["protocol"])
	assert.Equal(t, float64(0), filters[1]["port"])
	var pipeline map[string]any
	if !assert.NoError(t, json.Unmarshal([]byte(envValue(t, ds, "FLP_CONFIG")), &pipeline)) {
		t.FailNow()
	}
	// Inspect the destination semantically without depending on serialized JSON order.
	stages, ok := pipeline["parameters"].([]any)
	if !assert.True(t, ok) {
		t.FailNow()
	}
	found := false
	for _, stage := range stages {
		stage := stage.(map[string]any)
		if stage["name"] == "send" {
			grpc := stage["write"].(map[string]any)["grpc"].(map[string]any)
			assert.Equal(t, "collector.netobserv-cli.svc.cluster.local", grpc["targetHost"])
			assert.Equal(t, float64(9999), grpc["targetPort"])
			found = true
		}
	}
	if !assert.True(t, found, "missing collector destination") {
		t.FailNow()
	}
}
func TestPacketFiltersYAML(t *testing.T) {
	objects := generatedResources(t, "packets", "--node-selector=netobserv:true", "--port=80")
	if !assert.Len(t, objects, 10) {
		t.FailNow()
	}
	checkBaseResources(t, objects)
	checkCollectorService(t, objects)
	ds := agent(t, objects)
	assert.Equal(t, "true", ds.Spec.Template.Spec.NodeSelector["netobserv"])
	var filters []map[string]any
	if !assert.NoError(t, json.Unmarshal([]byte(envValue(t, ds, "FLOW_FILTER_RULES")), &filters)) {
		t.FailNow()
	}
	if !assert.Len(t, filters, 1) {
		t.FailNow()
	}
	assert.Equal(t, float64(80), filters[0]["port"])
}
func TestPacketOpenSSLYAML(t *testing.T) {
	ds := agent(t, generatedResources(t, "packets", "--port=8443", "--enable_openssl", "--tls_process_allowlist=nginx"))
	assert.Equal(t, "true", envValue(t, ds, "ENABLE_OPENSSL_TRACKING"))
	assert.Equal(t, "nginx", envValue(t, ds, "TLS_PLAINTEXT_PROCESS_ALLOWLIST"))
	assert.True(t, ds.Spec.Template.Spec.HostPID)
	c := ds.Spec.Template.Spec.Containers[0]
	if !assert.NotNil(t, c.SecurityContext) {
		t.FailNow()
	}
	if !assert.NotNil(t, c.SecurityContext.Privileged) {
		t.FailNow()
	}
	assert.True(t, *c.SecurityContext.Privileged)
	assert.Contains(t, c.SecurityContext.Capabilities.Add, corev1.Capability("SYS_PTRACE"))
	assert.Contains(t, c.VolumeMounts, corev1.VolumeMount{Name: "host-usr", MountPath: "/host/usr", ReadOnly: true})
	var filters []map[string]any
	if !assert.NoError(t, json.Unmarshal([]byte(envValue(t, ds, "FLOW_FILTER_RULES")), &filters)) {
		t.FailNow()
	}
	if !assert.Len(t, filters, 1) {
		t.FailNow()
	}
	assert.Equal(t, float64(8443), filters[0]["port"])
}
func TestMetricYAML(t *testing.T) {
	objects := generatedResources(t, "metrics")
	if !assert.Len(t, objects, 14) {
		t.FailNow()
	}
	checkBaseResources(t, objects)
	resource(t, objects, "ClusterRole", "netobserv-cli-metrics")
	binding := &rbacv1.ClusterRoleBinding{}
	if !assert.NoError(t, runtime.DefaultUnstructuredConverter.FromUnstructured(resource(t, objects, "ClusterRoleBinding", "netobserv-cli-metrics").Object, binding)) {
		t.FailNow()
	}
	assert.Equal(t, "netobserv-cli-metrics", binding.RoleRef.Name)
	assert.Equal(t, []rbacv1.Subject{{Kind: "ServiceAccount", Name: "prometheus-k8s", Namespace: "openshift-monitoring"}}, binding.Subjects)
	monitor := resource(t, objects, "ServiceMonitor", "netobserv-cli")
	namespaces, _, err := unstructured.NestedStringSlice(monitor.Object, "spec", "namespaceSelector", "matchNames")
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	assert.Equal(t, []string{"netobserv-cli"}, namespaces)
	selector, _, err := unstructured.NestedStringMap(monitor.Object, "spec", "selector", "matchLabels")
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	assert.Equal(t, "netobserv-cli", selector["app"])
	service := &corev1.Service{}
	if !assert.NoError(t, runtime.DefaultUnstructuredConverter.FromUnstructured(resource(t, objects, "Service", "netobserv-cli").Object, service)) {
		t.FailNow()
	}
	if !assert.Len(t, service.Spec.Ports, 1) {
		t.FailNow()
	}
	assert.Equal(t, int32(9401), service.Spec.Ports[0].Port)
	assert.Equal(t, int32(9401), service.Spec.Ports[0].TargetPort.IntVal)
	dashboard := resource(t, objects, "ConfigMap", "netobserv-cli")
	assert.Equal(t, "openshift-config-managed", dashboard.GetNamespace())
	assert.Equal(t, "true", dashboard.GetLabels()["console.openshift.io/dashboard"])
	assert.Contains(t, agent(t, objects).Spec.Template.Spec.Containers[0].Ports, corev1.ContainerPort{Name: "prometheus", ContainerPort: 9401, Protocol: corev1.ProtocolTCP})
}
