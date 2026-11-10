// Command plugin is the oc/kubectl client plugin entrypoint, replacing commands/netobserv.
// It is built twice under two names (oc-netobserv, kubectl-netobserv) by `make commands`; the
// only difference is cosmetic help text, resolved at runtime from the invoked name.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	commands "github.com/netobserv/network-observability-cli/internal/plugin/commands"
)

// version is overridden at build time via -ldflags "-X main.version=...", the same mechanism the
// Makefile already uses for the in-pod binary's main.buildVersion/main.buildDate.
var version = "main"

func main() {
	os.Exit(run())
}

func run() int {
	// Catching SIGINT/SIGTERM ourselves (rather than letting the Go runtime's default handler
	// kill the process outright) is what makes internal/plugin's `defer onCaptureExit(...)`
	// still run on Ctrl+C, matching commands/netobserv's `trap onExit EXIT`, which fired on
	// signals too: without this, interrupting a capture would orphan its namespace/DaemonSet/
	// collector Pod on the cluster instead of prompting to copy output and cleaning up.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	root := commands.NewRootCommand(os.Args[0], version, os.Args[1:])
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
