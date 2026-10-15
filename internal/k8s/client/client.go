// Package k8sclient builds a Kubernetes/OpenShift client the same way oc and kubectl do
// (respecting $KUBECONFIG, --kubeconfig and the current context) and provides a single,
// typed-or-unstructured Server-Side Apply path used in place of `oc apply -f -`.
package k8sclient

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Client bundles the three client-go entry points needed to replace oc/kubectl shell-outs:
// a typed clientset for well-known resources, a dynamic client for CRDs/arbitrary manifests
// (the project deliberately avoids generated OpenShift clientsets, see internal/k8s/tls),
// and discovery/REST mapping for Server-Side Apply against arbitrary GVKs.
type Client struct {
	RESTConfig *rest.Config
	Clientset  kubernetes.Interface
	Dynamic    dynamic.Interface
	Discovery  discovery.DiscoveryInterface
}

// New loads a kubeconfig the same way oc/kubectl do: explicit path if given, else
// $KUBECONFIG, else ~/.kube/config, overridden by kubeContext if non-empty.
func New(kubeconfigPath, kubeContext string) (*Client, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfigPath != "" {
		rules.ExplicitPath = kubeconfigPath
	}
	overrides := &clientcmd.ConfigOverrides{}
	if kubeContext != "" {
		overrides.CurrentContext = kubeContext
	}

	restConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("cannot load kubeconfig: %w", err)
	}
	return NewForConfig(restConfig)
}

// NewInCluster builds a client from the service-account credentials mounted in
// a Kubernetes pod. It is used by the collector binary, which runs inside the
// collector pod and does not have a kubeconfig.
func NewInCluster() (*Client, error) {
	restConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("cannot load in-cluster Kubernetes config: %w", err)
	}
	return NewForConfig(restConfig)
}

// NewForConfig builds a Client from an already-resolved rest.Config. Exposed separately so
// tests and in-cluster callers can bypass kubeconfig loading.
func NewForConfig(restConfig *rest.Config) (*Client, error) {
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("cannot create Kubernetes clientset: %w", err)
	}
	dyn, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("cannot create dynamic client: %w", err)
	}
	return &Client{
		RESTConfig: restConfig,
		Clientset:  clientset,
		Dynamic:    dyn,
		Discovery:  clientset.Discovery(),
	}, nil
}

// IsReachable replaces `oc get all 2>&1 | grep "Unable to connect"`: a plain connectivity/auth
// probe against the API server.
func (c *Client) IsReachable() error {
	if _, err := c.Discovery.ServerVersion(); err != nil {
		return fmt.Errorf("cluster is not reachable: %w", err)
	}
	return nil
}

// NamespaceExists replaces `oc get namespace $ns --ignore-not-found=true | grep -q $ns`.
func (c *Client) NamespaceExists(ctx context.Context, name string) (bool, error) {
	_, err := c.Clientset.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("cannot get namespace %q: %w", name, err)
	}
	return true, nil
}

// IgnoreNotFound mirrors `--ignore-not-found=true`: callers wrap every delete call with it so a
// missing resource is not an error, matching today's idempotent cleanup behavior.
func IgnoreNotFound(err error) error {
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}
