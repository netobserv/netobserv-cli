package capture

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/netobserv/network-observability-cli/internal/k8s/client"
)

func newFakeK8sClient() *k8sclient.Client {
	return &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset()}
}

func TestCreateCollectorPodForeground(t *testing.T) {
	client := newFakeK8sClient()

	pod, err := CreateCollectorPod(context.Background(), client, &CollectorPodOptions{
		Namespace:      "netobserv-cli",
		Image:          "quay.io/netobserv/network-observability-cli:main",
		ServiceAccount: "netobserv-cli",
		Command:        []string{"sleep", "infinity"},
	})
	require.NoError(t, err)

	assert.Equal(t, "collector", pod.Name)
	assert.Equal(t, "collector", pod.Labels["run"], "must match the collector Service's `run: collector` selector")
	assert.Equal(t, "netobserv-cli", pod.Spec.ServiceAccountName)
	require.Len(t, pod.Spec.Containers, 1)
	assert.Equal(t, []string{"sleep", "infinity"}, pod.Spec.Containers[0].Command)
	assert.Equal(t, corev1.PullAlways, pod.Spec.Containers[0].ImagePullPolicy, "defaults to Always when unset, matching bash's hardcoded --image-pull-policy=Always")
	assert.Empty(t, pod.Spec.InitContainers)
	require.Len(t, pod.Spec.Volumes, 1)
	assert.Equal(t, "output", pod.Spec.Volumes[0].Name)
}

func TestCreateCollectorPodHonorsConfiguredPullPolicy(t *testing.T) {
	client := newFakeK8sClient()

	pod, err := CreateCollectorPod(context.Background(), client, &CollectorPodOptions{
		Namespace:  "netobserv-cli",
		Image:      "localhost/netobserv-cli:test",
		PullPolicy: corev1.PullNever,
		Command:    []string{"sleep", "infinity"},
	})
	require.NoError(t, err)

	require.Len(t, pod.Spec.Containers, 1)
	assert.Equal(t, corev1.PullNever, pod.Spec.Containers[0].ImagePullPolicy,
		"local/kind-loaded test images aren't in a real registry, so e2e builds must be able to override Always")
}

func TestCreateCollectorPodWithTLS(t *testing.T) {
	client := newFakeK8sClient()

	pod, err := CreateCollectorPod(context.Background(), client, &CollectorPodOptions{
		Namespace:  "netobserv-cli",
		Image:      "quay.io/netobserv/network-observability-cli:main",
		Command:    []string{"sleep", "infinity"},
		TLSEnabled: true,
	})
	require.NoError(t, err)

	require.Len(t, pod.Spec.InitContainers, 1)
	assert.Equal(t, "resolve-tls", pod.Spec.InitContainers[0].Name)
	assert.Contains(t, pod.Spec.InitContainers[0].Command, "netobserv-cli")

	require.Len(t, pod.Spec.Volumes, 2)
	assert.Equal(t, "collector-tls", pod.Spec.Volumes[1].Name)
	require.NotNil(t, pod.Spec.Volumes[1].Secret)
	assert.Equal(t, "collector-tls", pod.Spec.Volumes[1].Secret.SecretName)

	c := pod.Spec.Containers[0]
	require.Len(t, c.EnvFrom, 1)
	assert.Equal(t, "collector-tls-config", c.EnvFrom[0].ConfigMapRef.Name)
	assert.False(t, *c.EnvFrom[0].ConfigMapRef.Optional)
	require.Len(t, c.VolumeMounts, 2)
	assert.Equal(t, "collector-tls", c.VolumeMounts[1].Name)
}
