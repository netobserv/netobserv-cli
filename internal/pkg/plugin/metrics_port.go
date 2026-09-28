package plugin

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const metricsReservationNamespace = "openshift-config-managed"

// Metrics agents use host networking. Reserve a port atomically across captures;
// namespace ownership releases the reservation when capture cleanup completes.
func (c *cluster) reserveMetricsPort(ctx context.Context, owner *metav1.OwnerReference) (int32, error) {
	pods, err := c.kube.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return 0, fmt.Errorf("list occupied metrics ports: %w", err)
	}
	occupied := map[int32]bool{}
	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		for j := range pod.Spec.Containers {
			container := &pod.Spec.Containers[j]
			for _, port := range container.Ports {
				if port.Protocol != "" && port.Protocol != corev1.ProtocolTCP {
					continue
				}
				if port.HostPort > 0 {
					occupied[port.HostPort] = true
				} else if pod.Spec.HostNetwork {
					occupied[port.ContainerPort] = true
				}
			}
		}
	}
	for port := int32(9401); port <= 9500; port++ {
		if occupied[port] {
			continue
		}
		claim := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("netobserv-cli-metrics-port-%d", port), Namespace: metricsReservationNamespace, OwnerReferences: []metav1.OwnerReference{*owner}}}
		if _, err := c.kube.CoreV1().ConfigMaps(metricsReservationNamespace).Create(ctx, claim, metav1.CreateOptions{}); err == nil {
			return port, nil
		} else if !apierrors.IsAlreadyExists(err) {
			return 0, fmt.Errorf("reserve metrics port: %w", err)
		}
	}
	return 0, fmt.Errorf("no available capture metrics port in range 9401-9500")
}

func (m *manifests) setMetricsPort(port int32) error {
	container := &m.agent.Spec.Template.Spec.Containers[0]
	for i := range container.Ports {
		if container.Ports[i].Name == "prometheus" {
			container.Ports[i].ContainerPort = port
			container.Ports[i].HostPort = port
		}
	}
	for i := range container.Env {
		env := &container.Env[i]
		if env.Name != "FLP_CONFIG" {
			continue
		}
		var pipeline map[string]any
		if err := json.Unmarshal([]byte(env.Value), &pipeline); err != nil {
			return err
		}
		settings, ok := pipeline["metricsSettings"].(map[string]any)
		if !ok {
			return fmt.Errorf("metrics pipeline has no metricsSettings")
		}
		settings["port"] = port
		data, err := json.Marshal(pipeline)
		if err != nil {
			return err
		}
		env.Value = string(data)
	}
	for _, obj := range m.objects {
		if obj.GetKind() != "Service" {
			continue
		}
		ports, _, err := unstructured.NestedSlice(obj.Object, "spec", "ports")
		if err != nil {
			return err
		}
		for _, entry := range ports {
			p := entry.(map[string]any)
			if p["name"] == "prometheus" {
				p["port"] = int64(port)
				p["targetPort"] = int64(port)
			}
		}
		if err := unstructured.SetNestedSlice(obj.Object, ports, "spec", "ports"); err != nil {
			return err
		}
	}
	return nil
}
