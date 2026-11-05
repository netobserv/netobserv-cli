package ocp

import (
	"context"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestDetectOpenShift(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()
	clientset.Resources = []*metav1.APIResourceList{
		{GroupVersion: "config.openshift.io/v1"},
	}

	isOCP, err := Detect(clientset.Discovery())
	require.NoError(t, err)
	assert.True(t, isOCP)
}

func TestDetectVanillaKubernetes(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()

	isOCP, err := Detect(clientset.Discovery())
	require.NoError(t, err)
	assert.False(t, isOCP)
}

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, configv1.Install(scheme))
	return scheme
}

func TestCompletedVersionUsesMostRecentHistoryEntry(t *testing.T) {
	cv := &configv1.ClusterVersion{
		Status: configv1.ClusterVersionStatus{
			History: []configv1.UpdateHistory{
				{State: configv1.CompletedUpdate, Version: "4.17.5"},
				{State: configv1.CompletedUpdate, Version: "4.16.10"},
			},
		},
	}
	version, ok := CompletedVersion(cv)
	require.True(t, ok)
	assert.Equal(t, "4.17.5", version)
}

func TestCompletedVersionStillUpgrading(t *testing.T) {
	cv := &configv1.ClusterVersion{
		Status: configv1.ClusterVersionStatus{
			History: []configv1.UpdateHistory{
				{State: configv1.PartialUpdate, Version: "4.18.0"},
				{State: configv1.CompletedUpdate, Version: "4.17.5"},
			},
		},
	}
	_, ok := CompletedVersion(cv)
	assert.False(t, ok)
}

func TestCompletedVersionEmptyHistory(t *testing.T) {
	_, ok := CompletedVersion(&configv1.ClusterVersion{})
	assert.False(t, ok)
}

func TestGetClusterVersion(t *testing.T) {
	scheme := newScheme(t)
	seed := &configv1.ClusterVersion{
		ObjectMeta: metav1.ObjectMeta{Name: "version"},
		Status: configv1.ClusterVersionStatus{
			History: []configv1.UpdateHistory{{State: configv1.CompletedUpdate, Version: "4.17.5"}},
		},
	}
	dyn := dynamicfake.NewSimpleDynamicClient(scheme, seed)

	cv, err := GetClusterVersion(context.Background(), dyn)
	require.NoError(t, err)
	version, ok := CompletedVersion(cv)
	require.True(t, ok)
	assert.Equal(t, "4.17.5", version)
}

func TestGetNetwork(t *testing.T) {
	scheme := newScheme(t)
	seed := &configv1.Network{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.NetworkSpec{
			ClusterNetwork: []configv1.ClusterNetworkEntry{{CIDR: "10.128.0.0/14"}},
			ServiceNetwork: []string{"172.30.0.0/16"},
		},
	}
	dyn := dynamicfake.NewSimpleDynamicClient(scheme, seed)

	network, err := GetNetwork(context.Background(), dyn)
	require.NoError(t, err)
	require.Len(t, network.Spec.ClusterNetwork, 1)
	assert.Equal(t, "10.128.0.0/14", network.Spec.ClusterNetwork[0].CIDR)
	assert.Equal(t, []string{"172.30.0.0/16"}, network.Spec.ServiceNetwork)
}

func TestConsoleURL(t *testing.T) {
	scheme := newScheme(t)
	seed := &configv1.Console{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Status:     configv1.ConsoleStatus{ConsoleURL: "https://console.example.com"},
	}
	dyn := dynamicfake.NewSimpleDynamicClient(scheme, seed)

	url, err := ConsoleURL(context.Background(), dyn)
	require.NoError(t, err)
	assert.Equal(t, "https://console.example.com", url)
}
