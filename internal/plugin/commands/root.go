package commands

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
)

var global globalOptions

// NewRootCommand builds the netobserv client plugin's Cobra command tree, replacing
// commands/netobserv's dispatch and scripts/help.sh's usage text. invokedAs is the program's own
// basename (e.g. "oc-netobserv" or "kubectl-netobserv"), used only to render the right "oc
// netobserv ..." / "kubectl netobserv ..." prefix in help text, the way a real kubectl plugin
// introspects its own name.
func NewRootCommand(invokedAs, version string, argv ...[]string) *cobra.Command {
	cliVersion = version
	pluginName := pluginInvocation(invokedAs)
	var rawArgs []string
	if len(argv) > 0 {
		rawArgs = append([]string(nil), argv[0]...)
	}

	root := &cobra.Command{
		Use:   "netobserv",
		Short: "Lightweight flow and packet visualization tool",
		Long:  fmt.Sprintf("%s deploys the NetObserv eBPF agent on your cluster to capture flows, packets or metrics for live viewing or local export.", pluginName),
		Example: fmt.Sprintf(`  Capture dropped flows:
    %[1]s flows --drops
  Capture flows from namespaces starting with "app-":
    %[1]s flows --query='SrcK8S_Namespace=~"app-.*"'
  Capture TCP flows on port 8080 or UDP flows:
    %[1]s flows --protocol=TCP --port=8080 or --protocol=UDP
  Capture TLS plaintext for a specific pod:
    %[1]s packets --port=8443 --enable_openssl --peer_ip=<pod-ip> --privileged --background
  Generate all available metrics:
    %[1]s metrics --enable_all`, pluginName),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.CompletionOptions.HiddenDefaultCmd = true

	root.PersistentFlags().StringVarP(&global.namespace, "namespace", "n", "", "Namespace to deploy the collector and agents in (default: netobserv-cli, or $NETOBSERV_NAMESPACE)")
	root.PersistentFlags().StringVar(&global.kubeconfig, "kubeconfig", "", "Path to the kubeconfig file to use (default: $KUBECONFIG, or ~/.kube/config)")
	root.PersistentFlags().StringVar(&global.kubeContext, "context", "", "Name of the kubeconfig context to use")

	root.AddCommand(
		newFlowsCommand(pluginName, argsAfterCommand(rawArgs, "flows")),
		newPacketsCommand(pluginName, argsAfterCommand(rawArgs, "packets")),
		newMetricsCommand(pluginName, argsAfterCommand(rawArgs, "metrics")),
		newFollowCommand(),
		newStopCommand(),
		newCopyCommand(),
		newCleanupCommand(),
		newVersionCommand(),
	)

	return root
}

// argsAfterCommand extracts the user's capture options from argv without
// making command handlers reach into the process-global os.Args. Global
// persistent flags may precede the subcommand; they are intentionally not
// forwarded to the in-pod display command.
func argsAfterCommand(argv []string, command string) []string {
	for i, arg := range argv {
		if arg == command {
			return append([]string(nil), argv[i+1:]...)
		}
	}
	return nil
}

func positionalHelpRequested(args []string) bool {
	return len(args) == 1 && args[0] == "help"
}

func pluginInvocation(invokedAs string) string {
	switch filepath.Base(invokedAs) {
	case "kubectl-netobserv", "kubectl":
		return "kubectl netobserv"
	default:
		return "oc netobserv"
	}
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the netobserv plugin version",
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "NetObserv CLI version %s\n", cliVersion)
			return nil
		},
	}
}
