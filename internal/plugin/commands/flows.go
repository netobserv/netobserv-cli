package commands

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	sharedcapture "github.com/netobserv/netobserv-operator/pkg/capture"
)

func newFlowsCommand(pluginName string, rawArgs []string) *cobra.Command {
	var capOpts captureOptions
	var featureFlags flowMetricsFlags
	var privileged bool

	cmd := &cobra.Command{
		Use:   "flows",
		Short: "Capture flows",
		Long: fmt.Sprintf(`Deploys the eBPF agent as a DaemonSet and streams captured flows to this terminal.

Flow filters narrow which flows are captured, e.g.:
  %[1]s flows --protocol=TCP --port=8080 or --protocol=UDP

Filters accepted: --direction, --cidr, --protocol, --sport, --dport, --port,
--sport_range, --dport_range, --port_range, --sports, --dports, --ports,
--tcp_flags, --drops, --icmp_type, --icmp_code, --peer_ip, --peer_cidr,
--action, --query. Separate independent OR-ed filter groups with a bare "or".`, pluginName),
		Example: fmt.Sprintf("  %[1]s flows\n  %[1]s flows --enable_all\n  %[1]s flows --protocol=TCP --port=8080 or --protocol=UDP", pluginName),
		Args:    cobra.ArbitraryArgs,
		// Flow-filter flags (--protocol, --port, ...) and the bare "or" separator are handled by
		// splitFilterArgs/applyFilterGroups from the explicit argv snapshot, not registered on
		// this FlagSet (see filters.go) — so Cobra must not error on them as "unknown flag".
		FParseErrWhitelist: cobra.FParseErrWhitelist{UnknownFlags: true},
		RunE: func(cmd *cobra.Command, args []string) error {
			if positionalHelpRequested(args) {
				return cmd.Help()
			}
			global.applyDefaults()
			if err := capOpts.validate("flows"); err != nil {
				return err
			}

			builder, err := newCaptureBuilder(sharedcapture.Flows, &global)
			if err != nil {
				return err
			}

			reqs, _, err := featureFlags.apply(builder)
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
			return runCapture(cmd, sharedcapture.Flows, &global, &capOpts, builder, reqs, rawArgs)
		},
	}

	registerCommonCaptureFlags(cmd, &capOpts, true, true, 5*time.Minute)
	registerFlowMetricsFlags(cmd, &featureFlags)
	registerPrivilegedFlag(cmd, &privileged)

	return cmd
}
