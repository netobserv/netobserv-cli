// Command kindcluster spins up the local kind cluster used for manual testing, replacing
// scripts/kind-cluster.sh. It still shells out to the external `kind` binary (reimplementing
// kind itself is out of scope), but the node-labeling step that bash did via `oc`/`kubectl label
// node` now goes through internal/k8s/client. Invoked via `make create-kind-cluster`; not
// shipped to users.
package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/netobserv/network-observability-cli/internal/k8s/client"
	"github.com/netobserv/network-observability-cli/internal/k8s/kindconfig"
)

const clusterName = "netobserv-cli-cluster"
const kindImage = "kindest/node:v1.32.0"

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	config := kindconfig.Manual(os.Getenv)
	configYAML, err := config.YAML()
	if err != nil {
		return err
	}

	kubeconfigPath, err := kubeconfigPath()
	if err != nil {
		return err
	}

	fmt.Printf("Creating kind cluster %q...\n", clusterName)
	//nolint:gosec // fixed command, no user-controlled arguments
	createCmd := exec.Command("kind", "create", "cluster",
		"--image", kindImage,
		"--config", "-",
		"--kubeconfig", kubeconfigPath,
		"--name", clusterName,
	)
	createCmd.Stdin = bytes.NewReader(configYAML)
	createCmd.Stdout = os.Stdout
	createCmd.Stderr = os.Stderr
	if err := createCmd.Run(); err != nil {
		return fmt.Errorf("kind create cluster failed: %w", err)
	}

	if err := labelWorkerNodes(kubeconfigPath); err != nil {
		return err
	}

	fmt.Println("Your kind cluster was created successfully")
	fmt.Println("Run the following to load the kubeconfig:")
	fmt.Printf("export KUBECONFIG=%s\n", kubeconfigPath)
	return nil
}

func kubeconfigPath() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return wd + "/kubeconfig", nil
}

// labelWorkerNodes replaces `oc/kubectl label node <name> node-role.kubernetes.io/worker=`: kind
// doesn't label its worker nodes with the conventional role label itself.
func labelWorkerNodes(kubeconfigPath string) error {
	client, err := k8sclient.New(kubeconfigPath, "")
	if err != nil {
		return fmt.Errorf("cannot connect to the new cluster: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, name := range []string{clusterName + "-worker", clusterName + "-worker2"} {
		patch := []byte(`{"metadata":{"labels":{"node-role.kubernetes.io/worker":""}}}`)
		if _, err := client.Clientset.CoreV1().Nodes().Patch(ctx, name, types.MergePatchType, patch, metav1.PatchOptions{}); err != nil {
			return fmt.Errorf("cannot label node %s: %w", name, err)
		}
	}
	return nil
}
