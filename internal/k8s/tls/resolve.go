package tlsresolver

import (
	"context"
	"fmt"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// ConfigMapName is the ConfigMap holding the resolved TLS_* settings, consumed via envFrom by both
// the collector pod and the agent DaemonSet.
const ConfigMapName = "collector-tls-config"

// apiServerGVR points to the cluster-scoped OpenShift APIServer resource that carries the
// tlsSecurityProfile.
var apiServerGVR = schema.GroupVersionResource{
	Group:    "config.openshift.io",
	Version:  "v1",
	Resource: "apiservers",
}

// Resolve reads the cluster's APIServer tlsSecurityProfile, converts it to numeric TLS settings and
// writes them to the collector-tls-config ConfigMap in namespace. When the cluster has no explicit
// profile it falls back to the Intermediate preset, mirroring the operator's runtime behavior.
func Resolve(ctx context.Context, namespace string) error {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return fmt.Errorf("cannot get Kubernetes InClusterConfig: %w", err)
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("cannot create dynamic client: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("cannot create Kubernetes client: %w", err)
	}

	profile, err := fetchTLSProfile(ctx, dyn)
	if err != nil {
		return err
	}

	// ComposeTLSConfig always returns a usable config; a non-nil error (e.g. unknown version) is
	// logged but must not prevent the resolved values from being written.
	tlsCfg, composeErr := ComposeTLSConfig(profile)
	if composeErr != nil {
		logrus.Warnf("TLS profile %q partially resolved: %v", profile.Type, composeErr)
	}

	data := ConfigToEnvMap(tlsCfg)
	logrus.Infof("Resolved TLS profile %q into %v", profile.Type, data)

	return applyConfigMap(ctx, clientset, namespace, data)
}

// fetchTLSProfile reads .spec.tlsSecurityProfile from the APIServer 'cluster' resource. It returns
// the Intermediate preset when the APIServer resource or the profile is absent (OCP's implicit
// default), so the resulting ConfigMap is always populated with concrete values.
func fetchTLSProfile(ctx context.Context, dyn dynamic.Interface) (*configv1.TLSSecurityProfile, error) {
	intermediate := &configv1.TLSSecurityProfile{Type: configv1.TLSProfileIntermediateType}

	u, err := dyn.Resource(apiServerGVR).Get(ctx, "cluster", metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return intermediate, nil
		}
		return nil, fmt.Errorf("cannot get APIServer 'cluster': %w", err)
	}

	spec, found, err := unstructured.NestedMap(u.Object, "spec", "tlsSecurityProfile")
	if err != nil {
		return nil, fmt.Errorf("cannot read tlsSecurityProfile: %w", err)
	}
	if !found || len(spec) == 0 {
		return intermediate, nil
	}

	var profile configv1.TLSSecurityProfile
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(spec, &profile); err != nil {
		return nil, fmt.Errorf("cannot decode tlsSecurityProfile: %w", err)
	}
	if profile.Type == "" {
		return intermediate, nil
	}
	return &profile, nil
}

// applyConfigMap creates or updates the collector-tls-config ConfigMap with the given data.
func applyConfigMap(ctx context.Context, clientset kubernetes.Interface, namespace string, data map[string]string) error {
	cms := clientset.CoreV1().ConfigMaps(namespace)

	existing, err := cms.Get(ctx, ConfigMapName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = cms.Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ConfigMapName,
				Namespace: namespace,
			},
			Data: data,
		}, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("cannot create ConfigMap %s: %w", ConfigMapName, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot get ConfigMap %s: %w", ConfigMapName, err)
	}

	existing.Data = data
	if _, err := cms.Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("cannot update ConfigMap %s: %w", ConfigMapName, err)
	}
	return nil
}
