package cleanup

import (
	"context"
	"fmt"

	"github.com/netobserv/network-observability-cli/internal/k8s/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const collectorDaemonSetName = "netobserv-cli"

func DeleteDaemonSet(ctx context.Context, namespace string) error {
	k8sClient, err := k8sclient.NewInCluster()
	if err != nil {
		return err
	}
	if err := k8sClient.Clientset.AppsV1().DaemonSets(namespace).Delete(
		ctx, collectorDaemonSetName, metav1.DeleteOptions{}); err != nil {
		return fmt.Errorf("cannot delete collector DaemonSet %q: %w", collectorDaemonSetName, err)
	}
	return nil
}
