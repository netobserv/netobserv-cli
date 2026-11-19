// Package ocp detects whether the connected cluster is OpenShift and reads the handful of
// cluster-scoped OpenShift config objects (ClusterVersion, Network, Console) the CLI needs,
// using the dynamic client decoded into openshift/api's typed structs — the same pattern
// internal/k8s/tls already uses, and the reason this project has never needed the
// generated openshift/client-go clientset as a dependency.
package ocp

import (
	"context"
	"fmt"

	configv1 "github.com/openshift/api/config/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
)

var (
	clusterVersionGVR = schema.GroupVersionResource{Group: "config.openshift.io", Version: "v1", Resource: "clusterversions"}
	networkGVR        = schema.GroupVersionResource{Group: "config.openshift.io", Version: "v1", Resource: "networks"}
	consoleGVR        = schema.GroupVersionResource{Group: "config.openshift.io", Version: "v1", Resource: "consoles"}
)

// Detect replaces checkClusterVersion's `oc get clusterversion` + "empty output means not
// OpenShift" heuristic with a direct discovery check for the config.openshift.io/v1 API group.
func Detect(disco discovery.DiscoveryInterface) (bool, error) {
	_, err := disco.ServerResourcesForGroupVersion("config.openshift.io/v1")
	if err == nil {
		return true, nil
	}
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	return false, fmt.Errorf("cannot check for OpenShift API group: %w", err)
}

// GetClusterVersion reads the cluster-scoped ClusterVersion "version" object.
func GetClusterVersion(ctx context.Context, dyn dynamic.Interface) (*configv1.ClusterVersion, error) {
	u, err := dyn.Resource(clusterVersionGVR).Get(ctx, "version", metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("cannot get ClusterVersion: %w", err)
	}
	cv := &configv1.ClusterVersion{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, cv); err != nil {
		return nil, fmt.Errorf("cannot decode ClusterVersion: %w", err)
	}
	return cv, nil
}

// CompletedVersion returns the cluster's current version string and true, or "", false if the
// cluster doesn't have a completed update yet (e.g. a fresh install still rolling out).
//
// Deviation from scripts/functions.sh's checkClusterVersion: that loop scanned the entire
// history for any entry with state "Completed" without breaking, so whichever matching entry
// came last in the (newest-first) array silently won — picking the *oldest* completed version
// once a cluster had upgraded more than once. status.history[0] is documented as the most
// recent entry, so checking only it is both simpler and the behavior OpenShift tooling
// conventionally relies on.
func CompletedVersion(cv *configv1.ClusterVersion) (string, bool) {
	if len(cv.Status.History) == 0 {
		return "", false
	}
	current := cv.Status.History[0]
	if current.State != configv1.CompletedUpdate {
		return "", false
	}
	return current.Version, true
}

// GetNetwork reads the cluster-scoped Network "cluster" object.
func GetNetwork(ctx context.Context, dyn dynamic.Interface) (*configv1.Network, error) {
	u, err := dyn.Resource(networkGVR).Get(ctx, "cluster", metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("cannot get Network: %w", err)
	}
	network := &configv1.Network{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, network); err != nil {
		return nil, fmt.Errorf("cannot decode Network: %w", err)
	}
	return network, nil
}

// ConsoleURL replaces `oc whoami --show-console`, reading the same cluster-scoped Console
// object the oc CLI itself resolves that URL from.
func ConsoleURL(ctx context.Context, dyn dynamic.Interface) (string, error) {
	u, err := dyn.Resource(consoleGVR).Get(ctx, "cluster", metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("cannot get Console: %w", err)
	}
	console := &configv1.Console{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, console); err != nil {
		return "", fmt.Errorf("cannot decode Console: %w", err)
	}
	return console.Status.ConsoleURL, nil
}
