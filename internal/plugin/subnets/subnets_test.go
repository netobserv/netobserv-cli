package subnets

import (
	"context"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

const installConfigYAML = `
networking:
  machineNetwork:
  - cidr: 10.0.0.0/16
  - cidr: fd00::/48
`

func TestDiscover(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-config-v1", Namespace: "kube-system"},
		Data:       map[string]string{"install-config": installConfigYAML},
	})

	scheme := runtime.NewScheme()
	require.NoError(t, configv1.Install(scheme))
	dyn := dynamicfake.NewSimpleDynamicClient(scheme, &configv1.Network{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.NetworkSpec{
			ClusterNetwork: []configv1.ClusterNetworkEntry{{CIDR: "10.128.0.0/14"}, {CIDR: "fd01::/48"}},
			ServiceNetwork: []string{"172.30.0.0/16", "fd02::/112"},
		},
	})

	result, err := Discover(context.Background(), clientset, dyn)
	require.NoError(t, err)

	assert.Equal(t, []string{"10.0.0.0/16", "fd00::/48"}, result[Machines])
	assert.Equal(t, []string{"10.128.0.0/14", "fd01::/48"}, result[Pods])
	assert.Equal(t, []string{"172.30.0.0/16", "fd02::/112"}, result[Services])
}

func TestDiscoverMissingConfigMap(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()
	scheme := runtime.NewScheme()
	require.NoError(t, configv1.Install(scheme))
	dyn := dynamicfake.NewSimpleDynamicClient(scheme)

	_, err := Discover(context.Background(), clientset, dyn)
	assert.Error(t, err)
}
