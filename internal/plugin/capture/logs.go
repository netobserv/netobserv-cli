package capture

import (
	"context"
	"fmt"
	"io"

	corev1 "k8s.io/api/core/v1"

	"github.com/netobserv/network-observability-cli/internal/k8s/client"
)

// FollowLogs streams a pod's logs to out until the context is canceled or the pod stops logging,
// replacing `oc logs <pod> -n <ns> -f`.
func FollowLogs(ctx context.Context, client *k8sclient.Client, namespace, pod string, out io.Writer) error {
	stream, err := client.Clientset.CoreV1().Pods(namespace).
		GetLogs(pod, &corev1.PodLogOptions{Follow: true}).
		Stream(ctx)
	if err != nil {
		return fmt.Errorf("cannot stream logs for pod %s/%s: %w", namespace, pod, err)
	}
	defer stream.Close()

	if _, err := io.Copy(out, stream); err != nil {
		return fmt.Errorf("error while streaming logs for pod %s/%s: %w", namespace, pod, err)
	}
	return nil
}
