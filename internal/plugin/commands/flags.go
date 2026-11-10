package commands

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/netobserv/network-observability-cli/internal/plugin/manifests/agent"
)

// registerCommonCaptureFlags registers the flags shared by flows/packets/metrics, replacing the
// common branches of parse_args (log-level, max-time, max-bytes, background, copy,
// node-selector, get-subnets, --yaml, --privileged).
func registerCommonCaptureFlags(cmd *cobra.Command, capOpts *captureOptions, allowMaxBytes, allowCopy bool, defaultMaxTime time.Duration) {
	flags := cmd.Flags()
	flags.StringVar(&capOpts.logLevel, "log-level", "info", "Log level of the agents and collector: trace, debug, info, warn, error, fatal, panic")
	flags.DurationVar(&capOpts.maxTime, "max-time", defaultMaxTime, "Automatically stop the capture after this duration")
	flags.BoolVar(&capOpts.background, "background", false, "Run the capture in the background instead of streaming it to this terminal")
	if allowCopy {
		flags.StringVar(&capOpts.copyMode, "copy", "prompt", `Whether to copy the capture output locally when it ends: "true", "false" or "prompt"`)
	} else {
		capOpts.copyMode = "false"
	}
	flags.StringVar(&capOpts.nodeSelector, "node-selector", "", "Restrict the agents to nodes matching this label, as key:value")
	flags.BoolVar(&capOpts.getSubnets, "get-subnets", false, "Label flows with the cluster's machine/pod/service subnets")
	flags.BoolVar(&capOpts.outputYAML, "yaml", false, "Write the manifests to ./output instead of applying them")

	if allowMaxBytes {
		flags.Int64Var(&capOpts.maxBytes, "max-bytes", 50_000_000, "Automatically stop the capture after this many bytes")
	}
}

func registerPrivilegedFlag(cmd *cobra.Command, privileged *bool) {
	cmd.Flags().BoolVar(privileged, "privileged", false, "Force privileged mode on the agents")
}

// flowMetricsFlags are the feature flags flows and metrics share (packets never supports them:
// parse_args rejected every one of these with "is invalid option for packets").
type flowMetricsFlags struct {
	sampling             int
	interfaces           []string
	excludeInterfaces    []string
	enablePktDrop        bool
	enableDNS            bool
	enableRTT            bool
	enableNetworkEvents  bool
	enableUDNMapping     bool
	enablePktTranslation bool
	enableIPSec          bool
	enableAll            bool
}

func registerFlowMetricsFlags(cmd *cobra.Command, f *flowMetricsFlags) {
	flags := cmd.Flags()
	flags.IntVar(&f.sampling, "sampling", 1, "eBPF sampling rate (1 = no sampling, capture everything)")
	flags.StringSliceVar(&f.interfaces, "interfaces", nil, "Interfaces to monitor (default: all); repeatable")
	flags.StringSliceVar(&f.excludeInterfaces, "exclude_interfaces", []string{"lo"}, "Interfaces to exclude; repeatable")
	flags.BoolVar(&f.enablePktDrop, "enable_pkt_drop", false, "Enable packet drop tracking (requires OpenShift 4.14+)")
	flags.BoolVar(&f.enableDNS, "enable_dns", false, "Enable DNS tracking")
	flags.BoolVar(&f.enableRTT, "enable_rtt", false, "Enable TCP RTT tracking")
	flags.BoolVar(&f.enableNetworkEvents, "enable_network_events", false, "Enable network events monitoring (requires OpenShift 4.19+)")
	flags.BoolVar(&f.enableUDNMapping, "enable_udn_mapping", false, "Enable User Defined Network mapping (requires OpenShift 4.18+)")
	flags.BoolVar(&f.enablePktTranslation, "enable_pkt_translation", false, "Enable packet translation tracking")
	flags.BoolVar(&f.enableIPSec, "enable_ipsec", false, "Enable IPsec tracking")
	flags.BoolVar(&f.enableAll, "enable_all", false, "Enable every optional feature above")
}

// apply replaces parse_args' enable_* branches and edit_manifest's matching cases. It returns
// the OpenShift version requirements the enabled features impose, and the extra Prometheus
// metric names they unlock (consumed only by the metrics command's --include_list handling;
// harmless to compute and discard for flows).
func (f *flowMetricsFlags) apply(b *agent.Builder) ([]versionRequirement, []string, error) {
	var reqs []versionRequirement
	var extraMetrics []string

	b.SetSampling(f.sampling)
	if len(f.interfaces) > 0 {
		b.SetInterfaces(f.interfaces)
	}
	if len(f.excludeInterfaces) > 0 {
		b.SetExcludeInterfaces(f.excludeInterfaces)
	}

	if f.enableAll {
		if err := b.EnableAll(); err != nil {
			return nil, nil, err
		}
		reqs = append(reqs,
			versionRequirement{feature: "Packet drops", minVersion: "4.14.0"},
			versionRequirement{feature: "Network events", minVersion: "4.19.0"},
			versionRequirement{feature: "UDN mapping", minVersion: "4.18.0"},
		)
		extraMetrics = append(extraMetrics,
			"workload_egress_bytes_total", "namespace_drop_packets_total",
			"namespace_dns_latency_seconds", "namespace_rtt_seconds",
			"namespace_network_policy_events_total")
		return reqs, extraMetrics, nil
	}

	if f.enablePktDrop {
		b.EnablePacketDrops()
		reqs = append(reqs, versionRequirement{feature: "Packet drops", minVersion: "4.14.0"})
		extraMetrics = append(extraMetrics, "workload_egress_bytes_total", "namespace_drop_packets_total")
	}
	if f.enableDNS {
		b.EnableDNSTracking()
		extraMetrics = append(extraMetrics, "namespace_dns_latency_seconds")
	}
	if f.enableRTT {
		b.EnableRTT()
		extraMetrics = append(extraMetrics, "namespace_rtt_seconds")
	}
	if f.enableNetworkEvents {
		b.EnableNetworkEvents()
		reqs = append(reqs, versionRequirement{feature: "Network events", minVersion: "4.19.0"})
		extraMetrics = append(extraMetrics, "namespace_network_policy_events_total")
	}
	if f.enableUDNMapping {
		if err := b.EnableUDNMapping(); err != nil {
			return nil, nil, err
		}
		reqs = append(reqs, versionRequirement{feature: "UDN mapping", minVersion: "4.18.0"})
	}
	if f.enablePktTranslation {
		b.EnablePacketTranslation()
	}
	if f.enableIPSec {
		b.EnableIPSecTracking()
	}

	return reqs, extraMetrics, nil
}
