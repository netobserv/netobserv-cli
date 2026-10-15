package manifests

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// BuildCollectorService returns the Service used by the collector pod.
func BuildCollectorService(namespace string) *corev1.Service {
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Service"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "collector",
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"run": "collector"},
			Ports: []corev1.ServicePort{{
				Name:       "collector",
				Protocol:   corev1.ProtocolTCP,
				Port:       9999,
				TargetPort: intstr.FromInt(9999),
			}},
		},
	}
}
