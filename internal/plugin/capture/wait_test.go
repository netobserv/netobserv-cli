package capture

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/netobserv/network-observability-cli/internal/k8s/client"
)

func TestWaitForPodReadySucceedsImmediatelyWhenAlreadyReady(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "collector", Namespace: "netobserv-cli"},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
		},
	})
	client := &k8sclient.Client{Clientset: clientset}

	err := WaitForPodReady(context.Background(), client, "netobserv-cli", "collector", 5*time.Second)
	assert.NoError(t, err)
}

func TestWaitForPodReadyTimesOutWhenNeverReady(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "collector", Namespace: "netobserv-cli"},
	})
	client := &k8sclient.Client{Clientset: clientset}

	err := WaitForPodReady(context.Background(), client, "netobserv-cli", "collector", 500*time.Millisecond)
	assert.Error(t, err)
}

func TestWaitForDaemonSetReadySucceeds(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(&appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: DaemonSetName, Namespace: "netobserv-cli"},
		Status:     appsv1.DaemonSetStatus{NumberReady: 3, DesiredNumberScheduled: 3},
	})
	client := &k8sclient.Client{Clientset: clientset}

	var progress bytes.Buffer
	err := WaitForDaemonSetReady(context.Background(), client, "netobserv-cli", DaemonSetName, 5*time.Second, &progress)
	require.NoError(t, err)
	assert.Contains(t, progress.String(), "3/3 Ready")
}

func TestWaitForDaemonSetReadyDoesNotPrintTransientZeroDesiredStatus(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(&appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: DaemonSetName, Namespace: "netobserv-cli"},
		Status:     appsv1.DaemonSetStatus{},
	})
	client := &k8sclient.Client{Clientset: clientset}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var progress bytes.Buffer
	err := WaitForDaemonSetReady(ctx, client, "netobserv-cli", DaemonSetName, time.Second, &progress)

	assert.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, progress.String())
}

func TestWaitForDaemonSetReadyBreaksEarlyOnCrashLoopBackOff(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{Name: DaemonSetName, Namespace: "netobserv-cli"},
			Status:     appsv1.DaemonSetStatus{NumberReady: 0, DesiredNumberScheduled: 1},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "netobserv-cli-abcde", Namespace: "netobserv-cli"},
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{{
					Name:  "netobserv-cli",
					State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
				}},
			},
		},
	)
	client := &k8sclient.Client{Clientset: clientset}

	start := time.Now()
	var progress bytes.Buffer
	err := WaitForDaemonSetReady(context.Background(), client, "netobserv-cli", DaemonSetName, time.Minute, &progress)
	elapsed := time.Since(start)

	assert.Error(t, err)
	assert.Less(t, elapsed, 10*time.Second, "CrashLoopBackOff must break out well before the timeout")
	assert.Contains(t, progress.String(), "CrashLoopBackOff")
}
