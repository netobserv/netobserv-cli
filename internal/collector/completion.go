package collector

import (
	"context"
	"encoding/json"
	"os"

	sharedcapture "github.com/netobserv/netobserv-operator/pkg/capture"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func markManagedCaptureCompleted() {
	if !optionEnabled("managed") {
		return
	}
	podName := os.Getenv("HOSTNAME")
	if podName == "" {
		log.Warn("Cannot mark capture completed: HOSTNAME is not set")
		return
	}
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Warnf("Cannot mark capture completed: %v", err)
		return
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Warnf("Cannot mark capture completed: %v", err)
		return
	}
	patch, err := json.Marshal(map[string]any{
		"metadata": map[string]any{
			"annotations": map[string]string{sharedcapture.CaptureCompletedAnnotation: "true"},
		},
	})
	if err != nil {
		log.Warnf("Cannot mark capture completed: %v", err)
		return
	}
	if _, err := clientset.CoreV1().Pods(namespace).Patch(context.Background(), podName, types.MergePatchType, patch, metav1.PatchOptions{}); err != nil {
		log.Warnf("Cannot mark capture completed: %v", err)
	}
}
