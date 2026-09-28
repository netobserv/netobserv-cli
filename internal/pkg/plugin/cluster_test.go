package plugin

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

func TestTLSManifests(t *testing.T) {
	t.Setenv("NETOBSERV_NAMESPACE", "my-capture")
	t.Setenv("NETOBSERV_COLLECTOR_IMAGE", "example/collector:test")
	t.Setenv("NETOBSERV_AGENT_IMAGE", "example/agent:test")
	o, err := parseOptions("flows", []string{"--enable_udn_mapping"})
	mustNoError(t, err)
	m, err := buildManifests(o, true, []any{map[string]any{"name": "Pods", "cidrs": []any{"10.0.0.0/8"}}})
	mustNoError(t, err)
	assert.Equal(t, "my-capture", m.pod.Namespace)
	assert.Equal(t, "example/collector:test", m.pod.Spec.Containers[0].Image)
	assert.Equal(t, []string{"/network-observability-cli", "resolve-tls", "--namespace", "my-capture"}, m.pod.Spec.InitContainers[0].Command)
	assert.False(t, *m.agent.Spec.Template.Spec.Containers[0].EnvFrom[0].ConfigMapRef.Optional)
	var config string
	for _, env := range m.agent.Spec.Template.Spec.Containers[0].Env {
		if env.Name == "FLP_CONFIG" {
			config = env.Value
		}
	}
	assert.Contains(t, config, `"caCertPath":"/etc/collector-ca/service-ca.crt"`)
	assert.Contains(t, config, `"subnetLabels"`)
	assert.Contains(t, config, `"secondaryNetworks"`)
	for _, obj := range m.objects {
		if obj.GetKind() == "Service" {
			assert.Equal(t, "collector-tls", obj.GetAnnotations()["service.beta.openshift.io/serving-cert-secret-name"])
		}
	}
}
func TestCleanupAndStop(t *testing.T) {
	ctx := context.Background()
	t.Run("unrelated namespace", func(t *testing.T) {
		k := fake.NewClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "other"}})
		c := &cluster{kube: k}
		assert.Error(t, c.cleanup(ctx, "other"))
		_, err := k.CoreV1().Namespaces().Get(ctx, "other", metav1.GetOptions{})
		mustNoError(t, err)
	})
	t.Run("unlabeled default namespace", func(t *testing.T) {
		k := fake.NewClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "netobserv-cli", UID: "legacy"}})
		c := &cluster{kube: k}
		mustNoError(t, c.cleanup(ctx, "netobserv-cli"))
		mustNoError(t, c.cleanup(ctx, "netobserv-cli"))
		var deleted bool
		for _, action := range k.Actions() {
			if action.Matches("delete", "namespaces") {
				deleted = true
				deleteAction := action.(ktesting.DeleteAction)
				assert.Equal(t, "netobserv-cli", deleteAction.GetName())
				assert.EqualValues(t, "legacy", *deleteAction.GetDeleteOptions().Preconditions.UID)
			}
		}
		assert.True(t, deleted)
	})
	t.Run("owned namespace", func(t *testing.T) {
		k := fake.NewClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "netobserv-cli", UID: "one", Labels: map[string]string{"app": "netobserv-cli"}}})
		c := &cluster{kube: k}
		mustNoError(t, c.stop(ctx, "netobserv-cli"))
		mustNoError(t, c.cleanup(ctx, "netobserv-cli"))
		mustNoError(t, c.cleanup(ctx, "netobserv-cli"))
	})
	t.Run("forbidden", func(t *testing.T) {
		k := fake.NewClientset()
		k.PrependReactor("delete", "daemonsets", func(ktesting.Action) (bool, runtime.Object, error) { return true, nil, errors.New("forbidden") })
		c := &cluster{kube: k}
		assert.ErrorContains(t, c.stop(ctx, "netobserv-cli"), "forbidden")
	})
}
func TestExistingNamespaceIsNeverCleanedUp(t *testing.T) {
	k := fake.NewClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "netobserv-cli", Labels: map[string]string{"app": "netobserv-cli"}}})
	c := &cluster{kube: k, discovery: k.Discovery()}
	o, err := parseOptions("flows", nil)
	mustNoError(t, err)
	assert.Error(t, c.run(context.Background(), o, strings.NewReader("no\n"), io.Discard, io.Discard))
	for _, a := range k.Actions() {
		assert.NotEqual(t, "delete", a.GetVerb())
	}
}
func TestCreateReconcilesLegacyRBAC(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "ClusterRole", "metadata": map[string]any{"name": "netobserv-cli"}}}
	d := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), obj)
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{{Group: "rbac.authorization.k8s.io", Version: "v1"}})
	mapper.Add(obj.GroupVersionKind(), meta.RESTScopeRoot)
	c := &cluster{dynamic: d, mapper: mapper}
	owner := metav1.OwnerReference{APIVersion: "v1", Kind: "Namespace", Name: "netobserv-cli", UID: "one"}
	mustNoError(t, c.create(context.Background(), obj.DeepCopy(), &owner))
	owner.UID = "two"
	assert.ErrorContains(t, c.create(context.Background(), obj.DeepCopy(), &owner), "another capture")
}

func TestSetupFailureCleansCreatedNamespace(t *testing.T) {
	k := fake.NewClientset()
	c := &cluster{kube: k, discovery: k.Discovery(), mapper: meta.NewDefaultRESTMapper(nil)}
	o, err := parseOptions("flows", nil)
	mustNoError(t, err)
	assert.Error(t, c.run(context.Background(), o, strings.NewReader("no\n"), io.Discard, io.Discard))
	namespaces, err := k.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{})
	mustNoError(t, err)
	assert.Empty(t, namespaces.Items)
}

func TestConcurrentMetricsDashboards(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	mustNoError(t, corev1.AddToScheme(scheme))
	d := dynamicfake.NewSimpleDynamicClient(scheme)
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{{Version: "v1"}})
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}, meta.RESTScopeNamespace)
	c := &cluster{dynamic: d, mapper: mapper}
	for _, ns := range []string{"capture-one", "capture-two"} {
		o, err := parseOptions("metrics", []string{"--namespace=" + ns})
		mustNoError(t, err)
		m, err := buildManifests(o, true, nil)
		mustNoError(t, err)
		found := false
		for _, obj := range m.objects {
			if obj.GetKind() == "ConfigMap" && obj.GetNamespace() == "openshift-config-managed" {
				found = true
				assert.Equal(t, ns, obj.GetName())
				owner := metav1.OwnerReference{APIVersion: "v1", Kind: "Namespace", Name: ns, UID: types.UID(ns)}
				mustNoError(t, c.create(ctx, obj, &owner))
			}
		}
		assert.True(t, found)
	}
	dashboards, err := d.Resource(schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}).Namespace("openshift-config-managed").List(ctx, metav1.ListOptions{})
	mustNoError(t, err)
	assert.Len(t, dashboards.Items, 2)
	objects := []runtime.Object{}
	for _, dashboard := range dashboards.Items {
		cm := &corev1.ConfigMap{}
		mustNoError(t, runtime.DefaultUnstructuredConverter.FromUnstructured(dashboard.Object, cm))
		objects = append(objects, cm)
		objects = append(objects, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: dashboard.GetName(), UID: types.UID(dashboard.GetName()), Labels: map[string]string{"app": "netobserv-cli"}}})
	}
	c.kube = fake.NewClientset(objects...)
	mustNoError(t, c.cleanup(ctx, "capture-one"))
	_, err = c.kube.CoreV1().ConfigMaps("openshift-config-managed").Get(ctx, "capture-two", metav1.GetOptions{})
	mustNoError(t, err)
	_, err = c.kube.CoreV1().ConfigMaps("openshift-config-managed").Get(ctx, "capture-one", metav1.GetOptions{})
	assert.True(t, apierrors.IsNotFound(err))
}

func TestCollectorPullPolicyDoesNotDisableAgentPulls(t *testing.T) {
	old := PullPolicy
	PullPolicy = "Never"
	t.Cleanup(func() { PullPolicy = old })
	for _, mode := range []string{"flows", "packets", "metrics"} {
		t.Run(mode, func(t *testing.T) {
			o, err := parseOptions(mode, []string{"--protocol=TCP"})
			mustNoError(t, err)
			m, err := buildManifests(o, false, nil)
			mustNoError(t, err)
			assert.Equal(t, corev1.PullNever, m.pod.Spec.Containers[0].ImagePullPolicy)
			assert.Equal(t, corev1.PullAlways, m.agent.Spec.Template.Spec.Containers[0].ImagePullPolicy)
		})
	}
}
