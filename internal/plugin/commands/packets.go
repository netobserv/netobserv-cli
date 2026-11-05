package commands

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	sharedcapture "github.com/netobserv/netobserv-operator/pkg/capture"
)

func newPacketsCommand(pluginName string, rawArgs []string) *cobra.Command {
	var capOpts captureOptions
	var privileged bool
	var enableOpenSSL bool
	var tlsPlaintextMinBytes int
	var tlsPlaintextPreviewBytes int
	var tlsPlaintextAllowlist []string

	cmd := &cobra.Command{
		Use:   "packets",
		Short: "Capture packets",
		Long: fmt.Sprintf(`Deploys the eBPF agent as a DaemonSet and streams captured packets to this terminal.

At least one flow filter is required, to avoid high resource consumption. See
"%[1]s flows --help" for the filter syntax (identical here).`, pluginName),
		Example:            fmt.Sprintf("  %[1]s packets --protocol=TCP --port=8080\n  %[1]s packets --enable_openssl --peer_ip=10.0.0.5", pluginName),
		Args:               cobra.ArbitraryArgs,
		FParseErrWhitelist: cobra.FParseErrWhitelist{UnknownFlags: true},
		RunE: func(cmd *cobra.Command, args []string) error {
			if positionalHelpRequested(args) {
				return cmd.Help()
			}
			global.applyDefaults()
			if err := capOpts.validate("packets"); err != nil {
				return err
			}

			builder, err := newCaptureBuilder(sharedcapture.Packets, &global)
			if err != nil {
				return err
			}

			if privileged {
				builder.EnablePrivileged()
			}
			if enableOpenSSL {
				builder.EnableOpenSSLTracking()
			}
			builder.SetTLSPlaintextMinBytes(tlsPlaintextMinBytes)
			builder.SetTLSPlaintextPreviewBytes(tlsPlaintextPreviewBytes)
			if len(tlsPlaintextAllowlist) > 0 {
				builder.SetTLSProcessAllowlist(tlsPlaintextAllowlist)
			}

			groups, queries, _ := splitFilterArgs(rawArgs)
			if err := applyFilterGroups(builder, groups); err != nil {
				return err
			}
			for _, q := range queries {
				builder.AddFilterQuery(q)
			}

			if noFiltersSet(groups) {
				return fmt.Errorf("at least one eBPF filter must be set for packet capture to avoid high resource consumption; see '%s packets --help'", pluginName)
			}

			warnPlaintextScope(cmd.OutOrStdout(), enableOpenSSL, groups)
			// isE2E mirrors bash's own check: a test harness often wraps the command in a real
			// PTY for realism, which term.IsTerminal alone can't tell apart from an actual user.
			interactive := term.IsTerminal(int(os.Stdin.Fd())) && os.Getenv("isE2E") != "true"
			if err := confirmTLSDecryptionLegal(cmd.OutOrStdout(), enableOpenSSL, interactive); err != nil {
				return err
			}

			return runCapture(cmd, sharedcapture.Packets, &global, &capOpts, builder, nil, rawArgs)
		},
	}

	registerCommonCaptureFlags(cmd, &capOpts, true, true, 5*time.Minute)
	registerPrivilegedFlag(cmd, &privileged)
	cmd.Flags().BoolVar(&enableOpenSSL, "enable_openssl", false, "Capture plaintext via libssl uprobes (hooks every container on each node; infra binaries excluded)")
	cmd.Flags().IntVar(&tlsPlaintextMinBytes, "tls_plaintext_min_bytes", 0, "Drop short TLS plaintext events before export")
	cmd.Flags().IntVar(&tlsPlaintextPreviewBytes, "tls_plaintext_preview_bytes", 256, "Plaintext preview length on exported events (0 = full payload)")
	cmd.Flags().StringSliceVar(&tlsPlaintextAllowlist, "tls_process_allowlist", nil, "Restrict TLS plaintext uprobes to these process names; repeatable")

	return cmd
}

func noFiltersSet(groups [][]filterToken) bool {
	for _, g := range groups {
		if len(g) > 0 {
			return false
		}
	}
	return true
}

// confirmTLSDecryptionLegal replaces confirm_tls_decryption_legal: TLS plaintext capture exposes
// encrypted traffic; require explicit interactive confirmation before proceeding (skipped for
// non-interactive callers, e.g. e2e tests, the same way isE2E did in bash).
func confirmTLSDecryptionLegal(out io.Writer, enableOpenSSL, interactive bool) error {
	if !enableOpenSSL || !interactive {
		return nil
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "WARNING: TLS decryption exposes encrypted traffic in plain text. In some")
	fmt.Fprintln(out, "         jurisdictions, capturing others' communications may be prohibited")
	fmt.Fprintln(out, "         without consent. Make sure this is legally permitted before proceeding.")
	fmt.Fprintln(out)
	if !promptYesNo(out, "Continue? [yes/no] ") {
		return fmt.Errorf("capture aborted")
	}
	return nil
}

// warnPlaintextScope replaces warn_plaintext_peer_scope: nudge users towards scoping an OpenSSL
// plaintext capture with --peer_ip/--peer_cidr and --port, rather than hooking every process on
// every node unscoped.
func warnPlaintextScope(out io.Writer, enableOpenSSL bool, groups [][]filterToken) {
	if !enableOpenSSL {
		return
	}
	hasPeerScope, hasPortFilter := false, false
	for _, group := range groups {
		for _, tok := range group {
			switch tok.key {
			case "peer_ip", "peer_cidr":
				hasPeerScope = true
			case "port", "sport", "dport", "ports", "sports", "dports", "port_range", "sport_range", "dport_range":
				hasPortFilter = true
			}
		}
	}
	if !hasPeerScope {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Warning: TLS plaintext capture has no --peer_ip or --peer_cidr scope.")
		fmt.Fprintln(out, "         --enable_openssl hooks libssl.so in every container on each node (infra binaries excluded).")
		fmt.Fprintln(out, "         Recommended: --peer_ip=<pod-ip> to limit which processes are hooked.")
	}
	if !hasPortFilter {
		fmt.Fprintln(out, "Warning: TLS plaintext capture has no --port (or --dport) filter.")
		fmt.Fprintln(out, "         Recommended: --port=<service-port> to filter exported plaintext and wire capture.")
	}
}
