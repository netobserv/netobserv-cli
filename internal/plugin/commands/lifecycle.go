package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/netobserv/network-observability-cli/internal/k8s/client"
	"github.com/netobserv/network-observability-cli/internal/plugin/capture"
)

func newFollowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "follow",
		Short: "Follow the progress of a background capture",
		RunE: func(cmd *cobra.Command, _ []string) error {
			global.applyDefaults()
			client, err := k8sclient.New(global.kubeconfig, global.kubeContext)
			if err != nil {
				return err
			}
			return capture.FollowLogs(cmd.Context(), client, global.namespace, capture.CollectorPodName, cmd.OutOrStdout())
		},
	}
}

func newStopCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop a running capture (removes the agent DaemonSet, keeps the collector and its output)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			global.applyDefaults()
			client, err := k8sclient.New(global.kubeconfig, global.kubeContext)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Deleting daemonset...")
			return capture.DeleteDaemonSet(cmd.Context(), client, global.namespace)
		},
	}
}

func newCopyCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "copy",
		Short: "Copy the capture output from the collector locally",
		RunE: func(cmd *cobra.Command, _ []string) error {
			global.applyDefaults()
			client, err := k8sclient.New(global.kubeconfig, global.kubeContext)
			if err != nil {
				return err
			}
			return doCopy(cmd.Context(), client, global.namespace, cmd.OutOrStdout())
		},
	}
}

func newCleanupCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "cleanup",
		Short: "Remove every netobserv-cli component from the cluster",
		RunE: func(cmd *cobra.Command, _ []string) error {
			global.applyDefaults()
			client, err := k8sclient.New(global.kubeconfig, global.kubeContext)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Cleaning up...")
			return capture.Cleanup(cmd.Context(), client, global.namespace, cmd.OutOrStdout())
		},
	}
}
