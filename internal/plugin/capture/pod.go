// Package capture orchestrates the ephemeral "collector" Pod and the capture DaemonSet against a
// live cluster: creating them, waiting for readiness, execing/streaming logs/copying files out of
// the collector, and tearing everything down. It replaces the oc/kubectl-calling parts of
// commands/netobserv and scripts/functions.sh (setup, waitDaemonset, follow, copyOutput, cleanup,
// deleteDaemonset/deletePod/deleteNamespace/deleteServiceMonitor/deleteDashboardCM) with typed
// client-go calls.
package capture

import (
	"context"
	"fmt"

	"github.com/netobserv/netobserv-operator/pkg/capture"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/netobserv/network-observability-cli/internal/k8s/client"
)

// CollectorPodName is the fixed name commands/netobserv has always used for the ephemeral
// collector Pod.
const CollectorPodName = "collector"

// CollectorPodOptions configures the ephemeral collector Pod, replacing the hand-escaped JSON
// string commands/netobserv built for `oc run --overrides=`.
type CollectorPodOptions struct {
	Namespace      string
	Image          string
	ServiceAccount string
	// PullPolicy replaces --image-pull-policy, which scripts/inject.sh templated in from the
	// Makefile's PULL_POLICY (defaulting to "Always" in production; "Never" for local/e2e builds
	// using an image that was `kind load`ed rather than pushed to a real registry).
	PullPolicy corev1.PullPolicy
	// Command is the collector container's entrypoint: ["sleep", "infinity"] for foreground
	// mode (the CLI execs the actual get-<kind> command into the pod afterwards), or
	// ["bash", "-c", "<get-<kind> invocation> && sleep infinity"] for --background/--yaml
	// pre-staged runs.
	Command []string
	// TLSEnabled adds the resolve-tls initContainer and wires the collector-tls Secret/
	// collector-tls-config ConfigMap, replacing commands/netobserv's TLS override branch.
	TLSEnabled bool
}

// CreateCollectorPod replaces `oc run collector --overrides=<json>`.
func CreateCollectorPod(ctx context.Context, client *k8sclient.Client, opts *CollectorPodOptions) (*corev1.Pod, error) {
	pullPolicy := opts.PullPolicy
	if pullPolicy == "" {
		pullPolicy = corev1.PullAlways
	}

	// The shared builder owns the common Pod shape and labels; CLI-only TLS wiring
	// is added below before the object is sent to the API server.
	pod := capture.BuildCollectorPod(capture.CollectorPodOptions{
		Name: CollectorPodName, Namespace: opts.Namespace, ServiceAccount: opts.ServiceAccount,
		Image: opts.Image, Command: opts.Command,
	})
	pod.Spec.Containers[0].ImagePullPolicy = pullPolicy

	if opts.TLSEnabled {
		// The resolve-tls initContainer writes the collector-tls-config ConfigMap (resolved
		// from the cluster's tlsSecurityProfile) before the collector starts; the collector
		// consumes it via envFrom so its gRPC server honors the cluster profile instead of
		// hardcoding a TLS version. Mirrors commands/netobserv's strategic-merge overrides.
		pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
			Name:         "collector-tls",
			VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "collector-tls"}},
		})
		pod.Spec.InitContainers = []corev1.Container{{
			Name:            "resolve-tls",
			Image:           opts.Image,
			ImagePullPolicy: pullPolicy,
			Command:         []string{"/network-observability-cli", "resolve-tls", "--namespace", opts.Namespace},
		}}
		pod.Spec.Containers[0].EnvFrom = append(pod.Spec.Containers[0].EnvFrom, corev1.EnvFromSource{
			ConfigMapRef: &corev1.ConfigMapEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: "collector-tls-config"},
				Optional:             ptr.To(false),
			},
		})
		pod.Spec.Containers[0].VolumeMounts = append(pod.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
			Name: "collector-tls", MountPath: "/etc/collector-tls", ReadOnly: true,
		})
	}

	created, err := client.Clientset.CoreV1().Pods(opts.Namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("cannot create collector pod: %w", err)
	}
	return created, nil
}
