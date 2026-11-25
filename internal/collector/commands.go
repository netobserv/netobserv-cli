package collector

import (
	"context"
	"time"

	tlsresolver "github.com/netobserv/network-observability-cli/internal/k8s/tls"
	"github.com/spf13/cobra"
)

func registerCollectorFlags() {
	flags := collectorRootCmd.PersistentFlags()
	flags.StringVarP(&logLevel, "loglevel", "l", "info", "Log level")
	flags.IntVarP(&port, "port", "", 9999, "TCP port to listen")
	flags.StringVarP(&filename, "filename", "", "", "Output file name")
	flags.StringVarP(&options, "options", "", "", "Options(s)")
	flags.DurationVarP(&maxTime, "maxtime", "", 5*time.Minute, "Maximum capture time")
	flags.Int64VarP(&maxBytes, "maxbytes", "", 50000000, "Maximum capture bytes")
	flags.StringVarP(&namespace, "namespace", "n", "netobserv-cli", "Namespace where agent pods are running")
	flags.BoolVarP(&useMocks, "mock", "", false, "Use mock")
}

func registerCollectorCommands() {
	collectorRootCmd.AddCommand(flowCmd, pktCmd, metricCmd, resolveTLSCmd)
}

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
