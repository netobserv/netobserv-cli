package capture

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/netobserv/network-observability-cli/internal/k8s/client"
)

// WaitForPodReady replaces `oc wait --timeout 120s --for=condition=Ready pod/<name>`.
func WaitForPodReady(ctx context.Context, client *k8sclient.Client, namespace, name string, timeout time.Duration) error {
	err := wait.PollUntilContextTimeout(ctx, 2*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		pod, err := client.Clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, nil //nolint:nilerr // transient API hiccups should not abort the wait; the timeout above still bounds it
		}
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("pod %s/%s did not become ready within %s: %w", namespace, name, timeout, err)
	}
	return nil
}

// WaitForDaemonSetReady replaces waitDaemonset: polls every 10s (same cadence as the bash
// implementation) until every desired pod is Ready, printing the same kind of progress line, and
// breaking early on CrashLoopBackOff rather than waiting out the full timeout. On failure it
// writes a diagnostic dump to progress (pod list, events, container states, recent logs) before
// returning an error.
func WaitForDaemonSetReady(ctx context.Context, client *k8sclient.Client, namespace, name string, timeout time.Duration, progress io.Writer) error {
	const interval = 10 * time.Second
	deadline := time.Now().Add(timeout)

	for {
		ds, err := client.Clientset.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err == nil {
			reasons, crashLooping := containerWaitingReasons(ctx, client, namespace)
			// The DaemonSet controller briefly reports 0 desired pods while it is
			// calculating the node set. Do not expose that transient 0/0 status;
			// the next poll will report 0/<node-count> once the desired count is
			// available.
			if ds.Status.DesiredNumberScheduled > 0 {
				fmt.Fprintf(progress, "%d/%d Ready. Reason(s): %s\n", ds.Status.NumberReady, ds.Status.DesiredNumberScheduled, joinSorted(reasons))
			}

			if ds.Status.DesiredNumberScheduled > 0 && ds.Status.NumberReady == ds.Status.DesiredNumberScheduled {
				return nil
			}
			if crashLooping {
				break
			}
		}

		if time.Now().After(deadline) {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}

	fmt.Fprintln(progress, "\nERROR: Daemonset pods failed to start within timeout")
	fmt.Fprintln(progress, "Collecting diagnostic information...")
	dumpDiagnostics(ctx, client, namespace, name, progress)
	return fmt.Errorf("daemonset %s/%s pods failed to start within %s", namespace, name, timeout)
}

// containerWaitingReasons replaces waitDaemonset's `get pods -o jsonpath=...waiting.reason`:
// the deduplicated, sorted set of every container's current waiting reason across the namespace,
// plus whether any of them is CrashLoopBackOff.
func containerWaitingReasons(ctx context.Context, client *k8sclient.Client, namespace string) ([]string, bool) {
	pods, err := client.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, false
	}
	reasons := map[string]bool{}
	crashLooping := false
	for i := range pods.Items {
		statuses := pods.Items[i].Status.ContainerStatuses
		for j := range statuses {
			if statuses[j].State.Waiting == nil {
				continue
			}
			reasons[statuses[j].State.Waiting.Reason] = true
			if statuses[j].State.Waiting.Reason == "CrashLoopBackOff" {
				crashLooping = true
			}
		}
	}
	out := make([]string, 0, len(reasons))
	for r := range reasons {
		out = append(out, r)
	}
	sort.Strings(out)
	return out, crashLooping
}

func joinSorted(items []string) string {
	return strings.Join(items, " ")
}

// dumpDiagnostics replaces waitDaemonset's failure-path diagnostics (pod list, events,
// descriptions, daemonset logs). It is best-effort: a failure fetching one piece of diagnostic
// information must not prevent the others from being printed.
func dumpDiagnostics(ctx context.Context, client *k8sclient.Client, namespace, daemonSetName string, out io.Writer) {
	fmt.Fprintln(out, "\n=== Pod Status ===")
	pods, err := client.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		fmt.Fprintf(out, "cannot list pods: %v\n", err)
	} else {
		for i := range pods.Items {
			fmt.Fprintf(out, "%s\t%s\t%s\n", pods.Items[i].Name, pods.Items[i].Status.Phase, pods.Items[i].Spec.NodeName)
		}
	}

	fmt.Fprintln(out, "\n=== Pod Events ===")
	events, err := client.Clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		fmt.Fprintf(out, "cannot list events: %v\n", err)
	} else {
		sort.Slice(events.Items, func(i, j int) bool {
			return events.Items[i].LastTimestamp.Before(&events.Items[j].LastTimestamp)
		})
		start := 0
		if len(events.Items) > 20 {
			start = len(events.Items) - 20
		}
		tail := events.Items[start:]
		for i := range tail {
			fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", tail[i].LastTimestamp, tail[i].InvolvedObject.Name, tail[i].Reason, tail[i].Message)
		}
	}

	fmt.Fprintln(out, "\n=== Container Waiting Reasons ===")
	for i := range pods.Items {
		statuses := pods.Items[i].Status.ContainerStatuses
		for j := range statuses {
			if statuses[j].State.Waiting != nil {
				fmt.Fprintf(out, "%s/%s: %s (%s), restarts=%d\n",
					pods.Items[i].Name, statuses[j].Name, statuses[j].State.Waiting.Reason, statuses[j].State.Waiting.Message, statuses[j].RestartCount)
			}
		}
	}

	fmt.Fprintln(out, "\n=== DaemonSet Logs (if available, last 50 lines) ===")
	if err := streamTailLogs(ctx, client, namespace, daemonSetName, 50, out); err != nil {
		fmt.Fprintf(out, "no logs available: %v\n", err)
	}
}

func streamTailLogs(ctx context.Context, client *k8sclient.Client, namespace, name string, tailLines int64, out io.Writer) error {
	pods, err := client.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app=" + name,
	})
	if err != nil || len(pods.Items) == 0 {
		return fmt.Errorf("no pods found for daemonset %s", name)
	}
	req := client.Clientset.CoreV1().Pods(namespace).GetLogs(pods.Items[0].Name, &corev1.PodLogOptions{TailLines: &tailLines})
	stream, err := req.Stream(ctx)
	if err != nil {
		return err
	}
	defer stream.Close()
	_, err = io.Copy(out, stream)
	return err
}
