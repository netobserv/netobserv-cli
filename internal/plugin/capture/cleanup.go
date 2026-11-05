package capture

import (
	"context"
	"fmt"
	"io"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/netobserv/network-observability-cli/internal/k8s/client"
)

// DaemonSetName, dashboardConfigMapNamespace etc. are the fixed resource names/namespaces
// commands/netobserv and scripts/functions.sh have always used.
const DaemonSetName = "netobserv-cli"

const dashboardConfigMapNamespace = "openshift-config-managed"

const (
	namespacePollInterval  = time.Second
	namespaceDeleteTimeout = 2 * time.Minute
	namespaceWarningAfter  = 30 * time.Second
)

var serviceMonitorGVR = schema.GroupVersionResource{Group: "monitoring.coreos.com", Version: "v1", Resource: "servicemonitors"}

// DeleteServiceMonitor replaces deleteServiceMonitor.
func DeleteServiceMonitor(ctx context.Context, client *k8sclient.Client, namespace string) error {
	err := client.Dynamic.Resource(serviceMonitorGVR).Namespace(namespace).Delete(ctx, DaemonSetName, metav1.DeleteOptions{})
	if err := k8sclient.IgnoreNotFound(err); err != nil {
		return fmt.Errorf("cannot delete ServiceMonitor: %w", err)
	}
	return nil
}

// DeleteDashboardConfigMap replaces deleteDashboardCM. Note the ConfigMap lives in
// openshift-config-managed, not the capture namespace.
func DeleteDashboardConfigMap(ctx context.Context, client *k8sclient.Client) error {
	err := client.Clientset.CoreV1().ConfigMaps(dashboardConfigMapNamespace).Delete(ctx, DaemonSetName, metav1.DeleteOptions{})
	if err := k8sclient.IgnoreNotFound(err); err != nil {
		return fmt.Errorf("cannot delete dashboard ConfigMap: %w", err)
	}
	return nil
}

// DeleteDaemonSet replaces deleteDaemonset, also what the `stop` subcommand calls on its own.
func DeleteDaemonSet(ctx context.Context, client *k8sclient.Client, namespace string) error {
	err := client.Clientset.AppsV1().DaemonSets(namespace).Delete(ctx, DaemonSetName, metav1.DeleteOptions{})
	if err := k8sclient.IgnoreNotFound(err); err != nil {
		return fmt.Errorf("cannot delete DaemonSet: %w", err)
	}
	return nil
}

// DeleteCollectorPod replaces deletePod.
func DeleteCollectorPod(ctx context.Context, client *k8sclient.Client, namespace string) error {
	err := client.Clientset.CoreV1().Pods(namespace).Delete(ctx, CollectorPodName, metav1.DeleteOptions{})
	if err := k8sclient.IgnoreNotFound(err); err != nil {
		return fmt.Errorf("cannot delete collector pod: %w", err)
	}
	return nil
}

func DeleteService(ctx context.Context, client *k8sclient.Client, namespace, name string) error {
	err := client.Clientset.CoreV1().Services(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err := k8sclient.IgnoreNotFound(err); err != nil {
		return fmt.Errorf("cannot delete Service %q: %w", name, err)
	}
	return nil
}

func DeleteConfigMap(ctx context.Context, client *k8sclient.Client, namespace, name string) error {
	err := client.Clientset.CoreV1().ConfigMaps(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err := k8sclient.IgnoreNotFound(err); err != nil {
		return fmt.Errorf("cannot delete ConfigMap %q: %w", name, err)
	}
	return nil
}

func DeleteSecret(ctx context.Context, client *k8sclient.Client, namespace, name string) error {
	err := client.Clientset.CoreV1().Secrets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err := k8sclient.IgnoreNotFound(err); err != nil {
		return fmt.Errorf("cannot delete Secret %q: %w", name, err)
	}
	return nil
}

func DeleteServiceAccount(ctx context.Context, client *k8sclient.Client, namespace, name string) error {
	err := client.Clientset.CoreV1().ServiceAccounts(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err := k8sclient.IgnoreNotFound(err); err != nil {
		return fmt.Errorf("cannot delete ServiceAccount %q: %w", name, err)
	}
	return nil
}

func DeleteRole(ctx context.Context, client *k8sclient.Client, namespace, name string) error {
	err := client.Clientset.RbacV1().Roles(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err := k8sclient.IgnoreNotFound(err); err != nil {
		return fmt.Errorf("cannot delete Role %q: %w", name, err)
	}
	return nil
}

func DeleteRoleBinding(ctx context.Context, client *k8sclient.Client, namespace, name string) error {
	err := client.Clientset.RbacV1().RoleBindings(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err := k8sclient.IgnoreNotFound(err); err != nil {
		return fmt.Errorf("cannot delete RoleBinding %q: %w", name, err)
	}
	return nil
}

// DeleteNamespace replaces deleteNamespace.
// DeleteNamespace waits for the namespace to be fully gone before returning, rather than just
// issuing the delete and moving on as `oc/kubectl delete --ignore-not-found` does: namespace
// deletion is asynchronous (finalizers can keep it in "Terminating" for a while), and
// namespaceExists (checked at the start of every capture) would otherwise see it as still
// existing, surfacing a confusing "netobserv-cli namespace already exists" if the user reruns a
// capture right after `netobserv cleanup`.
func DeleteNamespace(ctx context.Context, client *k8sclient.Client, namespace string) error {
	return deleteNamespace(ctx, client, namespace, nil)
}

func deleteNamespace(ctx context.Context, client *k8sclient.Client, namespace string, out io.Writer) error {
	err := client.Clientset.CoreV1().Namespaces().Delete(ctx, namespace, metav1.DeleteOptions{})
	if err := k8sclient.IgnoreNotFound(err); err != nil {
		return fmt.Errorf("cannot delete namespace %q: %w", namespace, err)
	}

	started := time.Now()
	warned := false
	err = wait.PollUntilContextTimeout(ctx, namespacePollInterval, namespaceDeleteTimeout, true, func(ctx context.Context) (bool, error) {
		ns, err := client.Clientset.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		if err == nil && out != nil && !warned && time.Since(started) >= namespaceWarningAfter {
			warned = true
			fmt.Fprintf(out, "\nWarning: namespace %q is still terminating after %s", namespace, time.Since(started).Round(time.Second))
			blockingCondition := false
			for _, condition := range ns.Status.Conditions {
				if condition.Status == corev1.ConditionTrue && condition.Message != "" {
					blockingCondition = true
					fmt.Fprintf(out, "\n  %s: %s", condition.Type, condition.Message)
				}
			}
			if !blockingCondition {
				fmt.Fprintln(out, "\n  No blocking namespace conditions reported; Kubernetes is completing finalization.")
			}
			fmt.Fprintln(out)
		}
		return false, nil //nolint:nilerr // keep polling on transient errors too; the timeout above still bounds it
	})
	if err != nil {
		return fmt.Errorf("namespace %q did not finish terminating: %w", namespace, err)
	}
	return nil
}

// Cleanup replaces cleanup's resource-teardown half (the copy-output-first prompt lives in
// internal/plugin, which calls CopyFromPod before Cleanup when appropriate), deleting
// resources in the same order bash did. Progress is written to out so foreground cleanup does
// not look stalled while Kubernetes processes the deletes, especially namespace finalizers.
func Cleanup(ctx context.Context, client *k8sclient.Client, namespace string, out io.Writer) error {
	steps := []struct {
		name string
		wait bool
		fn   func() error
	}{
		{"ServiceMonitor", false, func() error { return DeleteServiceMonitor(ctx, client, namespace) }},
		{"dashboard ConfigMap", false, func() error { return DeleteDashboardConfigMap(ctx, client) }},
		{"DaemonSet", false, func() error { return DeleteDaemonSet(ctx, client, namespace) }},
		{"collector Pod", false, func() error { return DeleteCollectorPod(ctx, client, namespace) }},
		{"collector Service", false, func() error { return DeleteService(ctx, client, namespace, CollectorPodName) }},
		{"metrics Service", false, func() error { return DeleteService(ctx, client, namespace, DaemonSetName) }},
		{"collector CA ConfigMap", false, func() error { return DeleteConfigMap(ctx, client, namespace, "collector-ca") }},
		{"collector TLS ConfigMap", false, func() error { return DeleteConfigMap(ctx, client, namespace, "collector-tls-config") }},
		{"collector TLS Secret", false, func() error { return DeleteSecret(ctx, client, namespace, "collector-tls") }},
		{"ServiceAccount", false, func() error { return DeleteServiceAccount(ctx, client, namespace, DaemonSetName) }},
		{"Role", false, func() error { return DeleteRole(ctx, client, namespace, DaemonSetName) }},
		{"RoleBinding", false, func() error { return DeleteRoleBinding(ctx, client, namespace, DaemonSetName) }},
		{"namespace", true, func() error { return deleteNamespace(ctx, client, namespace, out) }},
	}

	for _, step := range steps {
		if step.wait {
			fmt.Fprintf(out, "- deleting %s (waiting for termination)... ", step.name)
		} else {
			fmt.Fprintf(out, "- deleting %s... ", step.name)
		}
		if err := step.fn(); err != nil {
			fmt.Fprintln(out, "failed")
			return err
		}
		fmt.Fprintln(out, "done")
	}
	return nil
}
