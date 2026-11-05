// Package kindconfig contains the shared Kind cluster configuration used by
// developer tooling and end-to-end tests. Cluster lifecycle remains owned by
// each caller.
package kindconfig

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	APIVersion string      `yaml:"apiVersion"`
	Kind       string      `yaml:"kind"`
	Networking *Networking `yaml:"networking,omitempty"`
	Nodes      []Node      `yaml:"nodes"`
}

type Networking struct {
	PodSubnet     string `yaml:"podSubnet,omitempty"`
	ServiceSubnet string `yaml:"serviceSubnet,omitempty"`
	IPFamily      string `yaml:"ipFamily,omitempty"`
}

type Node struct {
	Role                 string        `yaml:"role"`
	ExtraMounts          []Mount       `yaml:"extraMounts,omitempty"`
	ExtraPortMappings    []PortMapping `yaml:"extraPortMappings,omitempty"`
	KubeadmConfigPatches []string      `yaml:"kubeadmConfigPatches,omitempty"`
}

type Mount struct {
	HostPath      string `yaml:"hostPath"`
	ContainerPath string `yaml:"containerPath"`
}

type PortMapping struct {
	ContainerPort int `yaml:"containerPort"`
	HostPort      int `yaml:"hostPort"`
}

// Manual returns the configuration used by the standalone kindcluster tool.
func Manual(getenv func(string) string) Config {
	valueOrDefault := func(name, fallback string) string {
		if value := getenv(name); value != "" {
			return value
		}
		return fallback
	}

	return Config{
		APIVersion: "kind.x-k8s.io/v1alpha4",
		Kind:       "Cluster",
		Networking: &Networking{
			PodSubnet:     valueOrDefault("NET_CIDR_IPV4", "10.244.0.0/16") + "," + valueOrDefault("NET_CIDR_IPV6", "fd00:10:244::/48"),
			ServiceSubnet: valueOrDefault("SVC_CIDR_IPV4", "10.96.0.0/16") + "," + valueOrDefault("SVC_CIDR_IPV6", "fd00:10:96::/112"),
			IPFamily:      valueOrDefault("IP_FAMILY", "dual"),
		},
		Nodes: manualNodes(),
	}
}

// E2E returns the minimal configuration used by the e2e framework. Tests access
// services from inside the cluster, so no host-port mapping is required.
func E2E() Config {
	return Config{
		APIVersion: "kind.x-k8s.io/v1alpha4",
		Kind:       "Cluster",
		Nodes:      []Node{{Role: "control-plane"}},
	}
}

func manualNodes() []Node {
	mount := Mount{HostPath: "/var/run/netns", ContainerPath: "/var/run/netns"}
	return []Node{
		{
			Role:        "control-plane",
			ExtraMounts: []Mount{mount},
			KubeadmConfigPatches: []string{`kind: ClusterConfiguration
apiServer:
    extraArgs:
        v: "5"
controllerManager:
    extraArgs:
        v: "5"
scheduler:
    extraArgs:
        v: "5"`},
		},
		{Role: "worker", ExtraMounts: []Mount{mount}},
		{Role: "worker", ExtraMounts: []Mount{mount}},
	}
}

func (c Config) YAML() ([]byte, error) {
	data, err := yaml.Marshal(c)
	if err != nil {
		return nil, fmt.Errorf("marshal kind configuration: %w", err)
	}
	return data, nil
}

// WriteTemp writes a configuration to a temporary file for APIs that accept a
// filename rather than YAML bytes, such as e2e-framework's Kind provider.
func WriteTemp(c Config) (string, error) {
	data, err := c.YAML()
	if err != nil {
		return "", err
	}
	f, err := os.CreateTemp("", "netobserv-kind-*.yaml")
	if err != nil {
		return "", fmt.Errorf("create temporary kind configuration: %w", err)
	}
	name := f.Name()
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(name)
		return "", fmt.Errorf("write temporary kind configuration: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(name)
		return "", fmt.Errorf("close temporary kind configuration: %w", err)
	}
	return name, nil
}
