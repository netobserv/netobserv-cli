package k8sclient

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestNamespaceExists(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(&corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "netobserv-cli"},
	})
	client := &Client{Clientset: clientset}

	exists, err := client.NamespaceExists(context.Background(), "netobserv-cli")
	require.NoError(t, err)
	assert.True(t, exists)

	exists, err = client.NamespaceExists(context.Background(), "does-not-exist")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestIsReachable(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()
	client := &Client{Discovery: clientset.Discovery()}

	assert.NoError(t, client.IsReachable())
}

func TestIgnoreNotFound(t *testing.T) {
	notFound := apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, "collector")
	assert.NoError(t, IgnoreNotFound(notFound))

	other := errors.New("boom")
	assert.Equal(t, other, IgnoreNotFound(other))

	assert.NoError(t, IgnoreNotFound(nil))
}
