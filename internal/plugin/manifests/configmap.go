package manifests

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CACertSecretName is the name of the serving-cert secret the collector Service is annotated
// to request on OpenShift (service.beta.openshift.io/serving-cert-secret-name).
const CACertSecretName = "collector-tls"

// CAConfigMapName is the ConfigMap the service-ca operator injects the resulting CA bundle
// into, mounted by the DaemonSet/collector for FLP client TLS.
const CAConfigMapName = "collector-ca"

// BuildCAConfigMap builds the ConfigMap that the service-ca operator injects the cluster CA
// bundle into (service.beta.openshift.io/inject-cabundle: "true"), replacing
// functions.sh's createCAConfigMap inline heredoc.
func BuildCAConfigMap(namespace string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      CAConfigMapName,
			Namespace: namespace,
			Annotations: map[string]string{
				"service.beta.openshift.io/inject-cabundle": "true",
			},
		},
	}
}
