// Package subnets discovers the cluster's machine/pod/service CIDRs, replacing
// scripts/functions.sh's getSubnets.
package subnets

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	sigsyaml "sigs.k8s.io/yaml"

	"github.com/netobserv/network-observability-cli/internal/k8s/ocp"
)

// Category names, used both as the returned map's keys and as the subnet label names injected
// into the FLP pipeline (NetworkTransformSubnetLabel.Name), matching scripts/functions.sh.
const (
	Machines = "Machines"
	Pods     = "Pods"
	Services = "Services"
)

// installConfig is the subset of the OpenShift install-config.yaml (itself stored as a YAML
// string inside the cluster-config-v1 ConfigMap) this package reads.
type installConfig struct {
	Networking struct {
		MachineNetwork []struct {
			CIDR string `json:"cidr"`
		} `json:"machineNetwork"`
	} `json:"networking"`
}

// Discover reads the cluster-config-v1 ConfigMap (kube-system) for machine network CIDRs and
// the OpenShift Network "cluster" object for pod/service network CIDRs.
//
// Deviation from scripts/functions.sh: there, each category was captured as a single
// newline-joined string, which its caller then spliced directly into a yq array-literal
// expression (`[${subnets[$key]}]`) relying on commas it never inserted — correct only when a
// category has exactly one CIDR, silently malformed on dual-stack clusters where pod/service
// networks have two. Here each category is a real []string, correct for any number of CIDRs.
func Discover(ctx context.Context, clientset kubernetes.Interface, dyn dynamic.Interface) (map[string][]string, error) {
	result := map[string][]string{}

	cm, err := clientset.CoreV1().ConfigMaps("kube-system").Get(ctx, "cluster-config-v1", metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("cannot get cluster-config-v1 ConfigMap: %w", err)
	}
	var ic installConfig
	if err := sigsyaml.Unmarshal([]byte(cm.Data["install-config"]), &ic); err != nil {
		return nil, fmt.Errorf("cannot decode install-config: %w", err)
	}
	for _, mn := range ic.Networking.MachineNetwork {
		if mn.CIDR != "" {
			result[Machines] = append(result[Machines], mn.CIDR)
		}
	}

	network, err := ocp.GetNetwork(ctx, dyn)
	if err != nil {
		return nil, err
	}
	for _, cn := range network.Spec.ClusterNetwork {
		if cn.CIDR != "" {
			result[Pods] = append(result[Pods], cn.CIDR)
		}
	}
	for _, sn := range network.Spec.ServiceNetwork {
		if sn != "" {
			result[Services] = append(result[Services], sn)
		}
	}

	return result, nil
}
