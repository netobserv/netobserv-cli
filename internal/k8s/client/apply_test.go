package k8sclient

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func newFakeClient(t *testing.T, objects ...runtime.Object) (*Client, *dynamicfake.FakeDynamicClient) {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	clientset := k8sfake.NewSimpleClientset()
	clientset.Resources = []*metav1.APIResourceList{
		{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{Name: "namespaces", Namespaced: false, Kind: "Namespace"},
				{Name: "configmaps", Namespaced: true, Kind: "ConfigMap"},
			},
		},
	}
	dyn := dynamicfake.NewSimpleDynamicClient(scheme, objects...)

	return &Client{Clientset: clientset, Dynamic: dyn, Discovery: clientset.Discovery()}, dyn
}

// capturedPatch records the single Patch call the fake dynamic client's "patch" reactor
// intercepts, so the test can assert on *how* ClusterSink calls the API (GVR, scope, patch
// type, field manager, force) without depending on the fake tracker's limited Server-Side
// Apply semantics (the stock fake tracker only emulates apply as an update to an object that
// already exists; real API servers create-on-apply, which is proven by the live e2e suite).
type capturedPatch struct {
	resource  schema.GroupVersionResource
	namespace string
	name      string
	patchType types.PatchType
	body      map[string]any
	options   metav1.PatchOptions
}

func interceptPatch(t *testing.T, dyn *dynamicfake.FakeDynamicClient) *capturedPatch {
	t.Helper()
	captured := &capturedPatch{}
	dyn.PrependReactor("patch", "*", func(action k8stesting.Action) (bool, runtime.Object, error) {
		p := action.(k8stesting.PatchActionImpl) //nolint:forcetypeassert // dynamic client patch actions are always this concrete type
		captured.resource = p.GetResource()
		captured.namespace = p.GetNamespace()
		captured.name = p.GetName()
		captured.patchType = p.GetPatchType()
		captured.options = p.PatchOptions
		require.NoError(t, json.Unmarshal(p.GetPatch(), &captured.body))

		return true, &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "v1",
			"kind":       captured.body["kind"],
			"metadata":   captured.body["metadata"],
		}}, nil
	})
	return captured
}

func TestClusterSinkApplyClusterScoped(t *testing.T) {
	client, dyn := newFakeClient(t)
	captured := interceptPatch(t, dyn)
	sink := &ClusterSink{Client: client}

	ns := &corev1.Namespace{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Namespace"},
		ObjectMeta: metav1.ObjectMeta{Name: "netobserv-cli", Labels: map[string]string{"app": "netobserv-cli"}},
	}

	require.NoError(t, sink.Apply(context.Background(), ns))

	assert.Equal(t, "namespaces", captured.resource.Resource)
	assert.Empty(t, captured.namespace)
	assert.Equal(t, "netobserv-cli", captured.name)
	assert.Equal(t, types.ApplyPatchType, captured.patchType)
	assert.Equal(t, FieldManager, captured.options.FieldManager)
	require.NotNil(t, captured.options.Force)
	assert.True(t, *captured.options.Force)
}

func TestClusterSinkApplyNamespaceScoped(t *testing.T) {
	client, dyn := newFakeClient(t)
	captured := interceptPatch(t, dyn)
	sink := &ClusterSink{Client: client}

	cm := &corev1.ConfigMap{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
		ObjectMeta: metav1.ObjectMeta{Name: "collector-ca", Namespace: "netobserv-cli"},
		Data:       map[string]string{"service-ca.crt": "dummy"},
	}

	require.NoError(t, sink.Apply(context.Background(), cm))

	assert.Equal(t, "configmaps", captured.resource.Resource)
	assert.Equal(t, "netobserv-cli", captured.namespace)
	assert.Equal(t, "collector-ca", captured.name)
	assert.Equal(t, types.ApplyPatchType, captured.patchType)
}

func TestClusterSinkApplyRejectsObjectWithoutGVK(t *testing.T) {
	client, _ := newFakeClient(t)
	sink := &ClusterSink{Client: client}

	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "no-gvk"}}

	err := sink.Apply(context.Background(), cm)
	assert.Error(t, err)
}

// TestClusterSinkApplyKeepsGoingPastAFailure replicates `kubectl apply -f -`'s behavior on a
// multi-document file: it applies every document it can, and only reports a failing one (e.g. a
// kind the cluster doesn't recognize, such as SecurityContextConstraints on non-OpenShift
// clusters) as part of the final aggregated error, rather than skipping everything after it.
func TestClusterSinkApplyKeepsGoingPastAFailure(t *testing.T) {
	client, dyn := newFakeClient(t)
	var appliedNames []string
	dyn.PrependReactor("patch", "*", func(action k8stesting.Action) (bool, runtime.Object, error) {
		p := action.(k8stesting.PatchActionImpl) //nolint:forcetypeassert // see interceptPatch
		appliedNames = append(appliedNames, p.GetName())
		return true, &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": p.GetName()}}}, nil
	})
	sink := &ClusterSink{Client: client}

	good1 := &corev1.Namespace{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Namespace"},
		ObjectMeta: metav1.ObjectMeta{Name: "before"},
	}
	bad := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "no-gvk"}}
	good2 := &corev1.Namespace{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Namespace"},
		ObjectMeta: metav1.ObjectMeta{Name: "after"},
	}

	err := sink.Apply(context.Background(), good1, bad, good2)
	require.Error(t, err)
	assert.Equal(t, []string{"before", "after"}, appliedNames)
}

func TestYAMLWriterApplySeparatesDocuments(t *testing.T) {
	var buf bytes.Buffer
	w := NewYAMLWriter(&buf)

	ns := &corev1.Namespace{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Namespace"},
		ObjectMeta: metav1.ObjectMeta{Name: "netobserv-cli"},
	}
	cm := &corev1.ConfigMap{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
		ObjectMeta: metav1.ObjectMeta{Name: "collector-ca", Namespace: "netobserv-cli"},
	}

	require.NoError(t, w.Apply(context.Background(), ns, cm))

	out := buf.String()
	assert.Contains(t, out, "kind: Namespace")
	assert.Contains(t, out, "kind: ConfigMap")
	assert.Equal(t, 1, countOccurrences(out, "---\n"))
}

func countOccurrences(s, sub string) int {
	count := 0
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			count++
		}
	}
	return count
}
