package agent

import (
	"fmt"

	flpapi "github.com/netobserv/flowlogs-pipeline/pkg/api"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

// EnableCollectorTLS replaces enableCollectorTLS: adds the CA volume/mount and
// collector-tls-config envFrom (the resolve-tls initContainer writes that ConfigMap from the
// cluster's TLS security profile, see internal/k8s/tls), and points the FLP pipeline's
// "send" (gRPC write) stage at the injected CA cert — all as typed fields instead of yq edits
// against a string that happens to contain JSON.
func (b *Builder) EnableCollectorTLS(caCertPath string) error {
	c := b.container()
	c.VolumeMounts = append(c.VolumeMounts, corev1.VolumeMount{
		Name: "collector-ca", MountPath: "/etc/collector-ca", ReadOnly: true,
	})
	b.ds.Spec.Template.Spec.Volumes = append(b.ds.Spec.Template.Spec.Volumes, corev1.Volume{
		Name: "collector-ca",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: "collector-ca"},
			},
		},
	})
	c.EnvFrom = append(c.EnvFrom, corev1.EnvFromSource{
		ConfigMapRef: &corev1.ConfigMapEnvSource{
			LocalObjectReference: corev1.LocalObjectReference{Name: "collector-tls-config"},
			Optional:             ptr.To(false),
		},
	})

	send, err := b.pipelineStage("send")
	if err != nil {
		return err
	}
	if send.Write == nil || send.Write.GRPC == nil {
		return fmt.Errorf("FLP 'send' stage has no grpc write configuration")
	}
	send.Write.GRPC.TLS = &flpapi.ClientTLS{CACertPath: caCertPath}
	return nil
}
