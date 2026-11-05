package capture

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/netobserv/network-observability-cli/internal/k8s/client"
)

func TestCleanupDeletesEverythingAndIgnoresNotFound(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "netobserv-cli"}},
		&appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: DaemonSetName, Namespace: "netobserv-cli"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: CollectorPodName, Namespace: "netobserv-cli"}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: CollectorPodName, Namespace: "netobserv-cli"}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: DaemonSetName, Namespace: "netobserv-cli"}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "collector-ca", Namespace: "netobserv-cli"}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "collector-tls-config", Namespace: "netobserv-cli"}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "collector-tls", Namespace: "netobserv-cli"}},
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: DaemonSetName, Namespace: "netobserv-cli"}},
		&rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: DaemonSetName, Namespace: "netobserv-cli"}},
		&rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: DaemonSetName, Namespace: "netobserv-cli"}},
		// no dashboard ConfigMap, no ServiceMonitor: Cleanup must tolerate their absence too.
	)
	client := &k8sclient.Client{Clientset: clientset, Dynamic: dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())}
	var output bytes.Buffer

	require.NoError(t, Cleanup(context.Background(), client, "netobserv-cli", &output))
	assert.Contains(t, output.String(), "- deleting ServiceMonitor... done")
	assert.Contains(t, output.String(), "- deleting namespace (waiting for termination)... done")

	_, err := clientset.AppsV1().DaemonSets("netobserv-cli").Get(context.Background(), DaemonSetName, metav1.GetOptions{})
	assert.True(t, apierrors.IsNotFound(err))

	_, err = clientset.CoreV1().Pods("netobserv-cli").Get(context.Background(), CollectorPodName, metav1.GetOptions{})
	assert.True(t, apierrors.IsNotFound(err))

	checks := []func() error{
		func() error {
			_, err := clientset.CoreV1().Services("netobserv-cli").Get(context.Background(), CollectorPodName, metav1.GetOptions{})
			return err
		},
		func() error {
			_, err := clientset.CoreV1().Services("netobserv-cli").Get(context.Background(), DaemonSetName, metav1.GetOptions{})
			return err
		},
		func() error {
			_, err := clientset.CoreV1().ConfigMaps("netobserv-cli").Get(context.Background(), "collector-ca", metav1.GetOptions{})
			return err
		},
		func() error {
			_, err := clientset.CoreV1().ConfigMaps("netobserv-cli").Get(context.Background(), "collector-tls-config", metav1.GetOptions{})
			return err
		},
		func() error {
			_, err := clientset.CoreV1().Secrets("netobserv-cli").Get(context.Background(), "collector-tls", metav1.GetOptions{})
			return err
		},
		func() error {
			_, err := clientset.CoreV1().ServiceAccounts("netobserv-cli").Get(context.Background(), DaemonSetName, metav1.GetOptions{})
			return err
		},
		func() error {
			_, err := clientset.RbacV1().Roles("netobserv-cli").Get(context.Background(), DaemonSetName, metav1.GetOptions{})
			return err
		},
		func() error {
			_, err := clientset.RbacV1().RoleBindings("netobserv-cli").Get(context.Background(), DaemonSetName, metav1.GetOptions{})
			return err
		},
	}
	for _, check := range checks {
		assert.True(t, apierrors.IsNotFound(check()))
	}

	_, err = clientset.CoreV1().Namespaces().Get(context.Background(), "netobserv-cli", metav1.GetOptions{})
	assert.True(t, apierrors.IsNotFound(err))
}

func TestDeleteDaemonSetIgnoresNotFound(t *testing.T) {
	client := &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset()}
	assert.NoError(t, DeleteDaemonSet(context.Background(), client, "netobserv-cli"))
}

func TestDeleteNamespaceIgnoresNotFound(t *testing.T) {
	client := &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset()}
	assert.NoError(t, DeleteNamespace(context.Background(), client, "netobserv-cli"))
}

// TestDeleteNamespaceWaitsForTermination replicates the race a plain delete-and-return would
// hit: a namespace stays gettable (e.g. "Terminating") for a little while after Delete is called,
// and a capture started right afterwards must not see it as still existing.
func TestDeleteNamespaceWaitsForTermination(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(&corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "netobserv-cli"},
	})
	client := &k8sclient.Client{Clientset: clientset}

	// The fake clientset's Delete removes the object immediately (no async finalizer
	// simulation), so "still Terminating" has to be synthesized here rather than relying on the
	// tracker's real state.
	getCount := 0
	clientset.PrependReactor("get", "namespaces", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		getCount++
		if getCount < 3 {
			return true, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "netobserv-cli"}}, nil
		}
		return true, nil, apierrors.NewNotFound(schema.GroupResource{Resource: "namespaces"}, "netobserv-cli")
	})

	require.NoError(t, DeleteNamespace(context.Background(), client, "netobserv-cli"))
	assert.GreaterOrEqual(t, getCount, 3)
}

func TestDeleteServiceMonitorIgnoresNotFound(t *testing.T) {
	client := &k8sclient.Client{Dynamic: dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())}
	assert.NoError(t, DeleteServiceMonitor(context.Background(), client, "netobserv-cli"))
}

func TestDeleteDashboardConfigMapIgnoresNotFound(t *testing.T) {
	client := &k8sclient.Client{Clientset: k8sfake.NewSimpleClientset()}
	assert.NoError(t, DeleteDashboardConfigMap(context.Background(), client))
}
