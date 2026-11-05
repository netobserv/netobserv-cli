// Package clicmd is the Cobra command tree for the netobserv client plugin, replacing
// commands/netobserv's dispatch and scripts/help.sh's usage text. Each command builds a
// internal/plugin/manifests/agent.Builder from internal/plugin/manifests templates, applies its flags to it,
// then drives the cluster through internal/plugin/capture and internal/k8s/client.
package commands

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Defaults, overridable per-build via -ldflags (see cmd/plugin/main.go, mirroring the
// LDFLAGS mechanism the Makefile already uses for main.buildVersion/main.buildDate) and at
// runtime via the same environment variables scripts/functions.sh read.
var (
	defaultCollectorImage = "quay.io/netobserv/network-observability-cli:main"
	defaultAgentImage     = "quay.io/netobserv/netobserv-ebpf-agent:main"
	// defaultPullPolicy replaces scripts/inject.sh's `sed -i "s/--image-pull-policy=.*/.../"`
	// substitution of the Makefile's PULL_POLICY into the collector Pod spec (e.g. "Never" for
	// e2e/dev builds using an image that was `kind load`ed rather than pushed to a registry).
	defaultPullPolicy = "Always"
	cliVersion        = "0.0.1"
)

const defaultNamespace = "netobserv-cli"

// globalOptions mirrors commands/netobserv's namespace/image resolution: a flag, falling back to
// an environment variable, falling back to a built-in default.
type globalOptions struct {
	namespace      string
	kubeconfig     string
	kubeContext    string
	collectorImage string
	agentImage     string
	pullPolicy     string
}

func (o *globalOptions) applyDefaults() {
	if o.namespace == "" {
		o.namespace = envOrDefault("NETOBSERV_NAMESPACE", defaultNamespace)
	}
	if o.collectorImage == "" {
		o.collectorImage = envOrDefault("NETOBSERV_COLLECTOR_IMAGE", defaultCollectorImage)
	}
	if o.agentImage == "" {
		o.agentImage = envOrDefault("NETOBSERV_AGENT_IMAGE", defaultAgentImage)
	}
	if o.pullPolicy == "" {
		o.pullPolicy = defaultPullPolicy
	}
}

func envOrDefault(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

// captureOptions are the flags shared by flows/packets/metrics, replacing the common branches of
// parse_args (log-level, max-time, max-bytes, background, copy, node-selector, get-subnets,
// --yaml).
type captureOptions struct {
	logLevel     string
	maxTime      time.Duration
	maxBytes     int64
	background   bool
	copyMode     string // "true", "false" or "prompt"
	nodeSelector string // "key:value"
	getSubnets   bool
	outputYAML   bool
}

func (o *captureOptions) validate(kind string) error {
	switch o.logLevel {
	case "trace", "debug", "info", "warn", "error", "fatal", "panic":
	default:
		return fmt.Errorf("invalid value for --log-level: %q", o.logLevel)
	}
	if o.copyMode != "true" && o.copyMode != "false" && o.copyMode != "prompt" {
		return fmt.Errorf("invalid value for --copy: %q", o.copyMode)
	}
	if o.nodeSelector != "" && !hasColon(o.nodeSelector) {
		return fmt.Errorf("invalid value for --node-selector: %q (use key:value)", o.nodeSelector)
	}
	if kind == "metrics" && o.maxBytes != 0 {
		return fmt.Errorf("--max-bytes is not a valid option for metrics")
	}
	return nil
}

func hasColon(s string) bool {
	return strings.ContainsRune(s, ':')
}

func splitNodeSelector(s string) (key, value string) {
	key, value, _ = strings.Cut(s, ":")
	return key, value
}
