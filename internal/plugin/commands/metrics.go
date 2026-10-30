package commands

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	sharedcapture "github.com/netobserv/netobserv-operator/pkg/capture"
)

// baseMetrics replaces help.sh's `includeList` default: the metrics always generated regardless
// of which optional features are enabled.
var baseMetrics = []string{
	"namespace_flows_total", "node_ingress_bytes_total", "node_egress_bytes_total", "workload_ingress_bytes_total",
}

func newMetricsCommand(pluginName string, rawArgs []string) *cobra.Command {
	var capOpts captureOptions
	var featureFlags flowMetricsFlags
	var privileged bool
	var includeList []string

	cmd := &cobra.Command{
		Use:                "metrics",
		Short:              "Generate on-demand Prometheus metrics",
		Long:               fmt.Sprintf("Deploys the eBPF agent as a DaemonSet exposing on-demand Prometheus metrics, scraped by the cluster monitoring stack and visualized via the %[1]s dashboard.", "netobserv-cli"),
		Example:            fmt.Sprintf("  %[1]s metrics\n  %[1]s metrics --enable_all --background", pluginName),
		Args:               cobra.ArbitraryArgs,
		FParseErrWhitelist: cobra.FParseErrWhitelist{UnknownFlags: true},
		RunE: func(cmd *cobra.Command, args []string) error {
			if positionalHelpRequested(args) {
				return cmd.Help()
			}
			global.applyDefaults()
			if err := capOpts.validate("metrics"); err != nil {
				return err
			}

			builder, err := newCaptureBuilder(sharedcapture.Metrics, &global)
			if err != nil {
				return err
			}

			reqs, extraMetrics, err := featureFlags.apply(builder)
			if err != nil {
				return err
			}
			if privileged {
				builder.EnablePrivileged()
			}

			groups, queries, _ := splitFilterArgs(rawArgs)
			if err := applyFilterGroups(builder, groups); err != nil {
				return err
			}
			for _, q := range queries {
				builder.AddFilterQuery(q)
			}

			restrictTo := append(append([]string{}, baseMetrics...), extraMetrics...)
			if cmd.Flags().Changed("include_list") {
				restrictTo = includeList
			}
			matched, err := builder.RestrictMetrics(restrictTo)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Matching metrics:")
			for _, m := range matched {
				fmt.Fprintf(cmd.OutOrStdout(), " - %s\n", m)
			}

			return runCapture(cmd, sharedcapture.Metrics, &global, &capOpts, builder, reqs, rawArgs)
		},
	}

	registerCommonCaptureFlags(cmd, &capOpts, false, false, time.Hour)
	registerFlowMetricsFlags(cmd, &featureFlags)
	registerPrivilegedFlag(cmd, &privileged)
	cmd.Flags().StringSliceVar(&includeList, "include_list", nil, "Restrict generated metrics to this explicit list, overriding feature-flag defaults; repeatable or comma-separated")

	return cmd
}
