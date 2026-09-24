package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

func TestMetricsPortsAreReservedConcurrently(t *testing.T) {
	k := fake.NewClientset(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "existing", Namespace: "other"}, Spec: corev1.PodSpec{HostNetwork: true, Containers: []corev1.Container{{Name: "metrics", Ports: []corev1.ContainerPort{{ContainerPort: 9401}}}}}}, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "netobserv-cli-metrics-port-9402", Namespace: metricsReservationNamespace}})
	c := &cluster{kube: k}
	ctx := context.Background()
	type result struct {
		port int32
		err  error
		uid  types.UID
	}
	results := make(chan result, 4)
	var wg sync.WaitGroup
	for i := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			uid := types.UID(fmt.Sprintf("capture-%d", i))
			p, e := c.reserveMetricsPort(ctx, &metav1.OwnerReference{APIVersion: "v1", Kind: "Namespace", Name: string(uid), UID: uid})
			results <- result{p, e, uid}
		}()
	}
	wg.Wait()
	close(results)
	seen := map[int32]bool{}
	for r := range results {
		mustNoError(t, r.err)
		assert.GreaterOrEqual(t, r.port, int32(9403))
		assert.False(t, seen[r.port])
		seen[r.port] = true
		cm, e := k.CoreV1().ConfigMaps(metricsReservationNamespace).Get(ctx, fmt.Sprintf("netobserv-cli-metrics-port-%d", r.port), metav1.GetOptions{})
		mustNoError(t, e)
		assert.Equal(t, r.uid, cm.OwnerReferences[0].UID)
	}
	assert.Len(t, seen, 4)
}

func TestMetricsPortExhaustion(t *testing.T) {
	objects := []runtime.Object{}
	for p := 9401; p <= 9500; p++ {
		objects = append(objects, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("netobserv-cli-metrics-port-%d", p), Namespace: metricsReservationNamespace}})
	}
	c := &cluster{kube: fake.NewClientset(objects...)}
	_, err := c.reserveMetricsPort(context.Background(), &metav1.OwnerReference{APIVersion: "v1", Kind: "Namespace", Name: "capture", UID: "capture"})
	assert.ErrorContains(t, err, "no available capture metrics port")
}

func TestReservedMetricsPortUpdatesListenerAndService(t *testing.T) {
	o, err := parseOptions("metrics", nil)
	mustNoError(t, err)
	m, err := buildManifests(o, true, nil)
	mustNoError(t, err)
	mustNoError(t, m.setMetricsPort(9417))
	c := m.agent.Spec.Template.Spec.Containers[0]
	assert.Equal(t, int32(9417), c.Ports[0].ContainerPort)
	assert.Equal(t, int32(9417), c.Ports[0].HostPort)
	for _, env := range c.Env {
		if env.Name == "FLP_CONFIG" {
			var pipeline map[string]any
			mustNoError(t, json.Unmarshal([]byte(env.Value), &pipeline))
			assert.Equal(t, float64(9417), pipeline["metricsSettings"].(map[string]any)["port"])
		}
	}
	for _, obj := range m.objects {
		if obj.GetKind() == "Service" {
			ports, _, err := unstructured.NestedSlice(obj.Object, "spec", "ports")
			mustNoError(t, err)
			p := ports[0].(map[string]any)
			assert.Equal(t, int64(9417), p["port"])
			assert.Equal(t, int64(9417), p["targetPort"])
		}
	}
}
