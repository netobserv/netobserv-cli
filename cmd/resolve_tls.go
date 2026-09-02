package cmd

import (
	"context"

	"github.com/netobserv/network-observability-cli/internal/pkg/tlsresolver"
	"github.com/spf13/cobra"
)

// resolveTLSCmd reads the cluster's OpenShift APIServer tlsSecurityProfile and writes the resolved
// numeric TLS settings to the collector-tls-config ConfigMap. It runs as the collector pod's
// initContainer so the ConfigMap exists before the collector server and the agents start.
var resolveTLSCmd = &cobra.Command{
	Use:   "resolve-tls",
	Short: "Resolve the cluster TLS security profile into a ConfigMap",
	Long: `Reads the OpenShift APIServer tlsSecurityProfile and writes the corresponding
TLS_MIN_VERSION / TLS_CIPHER_SUITES / TLS_CURVE_PREFERENCES settings to the
collector-tls-config ConfigMap, so the collector and agents honor the cluster profile.`,
	Run: runResolveTLS,
}

func runResolveTLS(_ *cobra.Command, _ []string) {
	if err := tlsresolver.Resolve(context.Background(), namespace); err != nil {
		log.Fatalf("failed to resolve TLS profile: %v", err)
	}
	log.Infof("TLS profile resolved into ConfigMap %s in namespace %s", tlsresolver.ConfigMapName, namespace)
}
