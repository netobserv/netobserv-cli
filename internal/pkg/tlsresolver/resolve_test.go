package tlsresolver

import (
	"context"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/netobserv/flowlogs-pipeline/pkg/tlsprofile"
)

func newFakeDynamic(objs ...runtime.Object) *dynamicfake.FakeDynamicClient {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{apiServerGVR: "APIServerList"},
		objs...,
	)
}

func apiServerWithProfile(t *testing.T, profileType string) *unstructured.Unstructured {
	t.Helper()
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(schema.GroupVersionKind{Group: "config.openshift.io", Version: "v1", Kind: "APIServer"})
	u.SetName("cluster")
	if profileType != "" {
		if err := unstructured.SetNestedField(u.Object, profileType, "spec", "tlsSecurityProfile", "type"); err != nil {
			t.Fatalf("failed to set profile type: %v", err)
		}
	}
	return u
}

func TestFetchTLSProfile(t *testing.T) {
	ctx := context.Background()

	t.Run("APIServer absent falls back to Intermediate", func(t *testing.T) {
		profile, err := fetchTLSProfile(ctx, newFakeDynamic())
		assert.NoError(t, err)
		assert.Equal(t, configv1.TLSProfileIntermediateType, profile.Type)
	})

	t.Run("no tlsSecurityProfile falls back to Intermediate", func(t *testing.T) {
		profile, err := fetchTLSProfile(ctx, newFakeDynamic(apiServerWithProfile(t, "")))
		assert.NoError(t, err)
		assert.Equal(t, configv1.TLSProfileIntermediateType, profile.Type)
	})

	t.Run("explicit Modern profile is returned", func(t *testing.T) {
		profile, err := fetchTLSProfile(ctx, newFakeDynamic(apiServerWithProfile(t, "Modern")))
		assert.NoError(t, err)
		assert.Equal(t, configv1.TLSProfileModernType, profile.Type)
	})
}

func TestApplyConfigMap(t *testing.T) {
	ctx := context.Background()
	ns := "netobserv-cli"

	t.Run("creates then updates the ConfigMap", func(t *testing.T) {
		clientset := k8sfake.NewSimpleClientset()

		err := applyConfigMap(ctx, clientset, ns, map[string]string{tlsprofile.EnvMinVersion: "771"})
		assert.NoError(t, err)

		cm, err := clientset.CoreV1().ConfigMaps(ns).Get(ctx, ConfigMapName, metav1.GetOptions{})
		assert.NoError(t, err)
		assert.Equal(t, "771", cm.Data[tlsprofile.EnvMinVersion])

		// second call updates in place
		err = applyConfigMap(ctx, clientset, ns, map[string]string{tlsprofile.EnvMinVersion: "772"})
		assert.NoError(t, err)

		cm, err = clientset.CoreV1().ConfigMaps(ns).Get(ctx, ConfigMapName, metav1.GetOptions{})
		assert.NoError(t, err)
		assert.Equal(t, "772", cm.Data[tlsprofile.EnvMinVersion])
	})
}
