package cmd

import (
	"crypto/tls"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	grpcCollector "github.com/netobserv/flowlogs-pipeline/pkg/pipeline/write/grpc"
	"github.com/netobserv/flowlogs-pipeline/pkg/tlsprofile"
)

const (
	collectorCertPath = "/etc/collector-tls/tls.crt"
	collectorKeyPath  = "/etc/collector-tls/tls.key"
)

func collectorTLSOptions() []grpcCollector.CollectorOption {
	return buildCollectorTLSOptions(collectorCertPath, collectorKeyPath)
}

// buildCollectorTLSOptions loads the given cert/key pair and returns the gRPC server options enabling
// TLS on the collector. It returns nil (TLS disabled) when either file is missing or the pair fails to
// load. The paths are parameters so this can be unit-tested against a temporary key pair.
func buildCollectorTLSOptions(certPath, keyPath string) []grpcCollector.CollectorOption {
	tlsConfig := buildCollectorTLSConfig(certPath, keyPath)
	if tlsConfig == nil {
		return nil
	}

	log.Info("TLS enabled for collector")
	return []grpcCollector.CollectorOption{
		grpcCollector.WithGRPCServerOptions(grpc.Creds(credentials.NewTLS(tlsConfig))),
	}
}

// buildCollectorTLSConfig loads the cert/key pair and builds the server tls.Config, honoring the
// cluster TLS security profile through tlsprofile.Apply (TLS_MIN_VERSION / TLS_CIPHER_SUITES /
// TLS_CURVE_PREFERENCES, populated by the resolve-tls initContainer). No TLS version is hardcoded:
// the effective profile is always what the cluster dictates. Returns nil (TLS disabled) when either
// file is missing or the pair fails to load.
func buildCollectorTLSConfig(certPath, keyPath string) *tls.Config {
	if _, err := os.Stat(certPath); err != nil {
		return nil
	}
	if _, err := os.Stat(keyPath); err != nil {
		return nil
	}

	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		log.Errorf("Failed to load TLS certificates: %v", err)
		return nil
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	applied, err := tlsprofile.Apply(tlsConfig)
	if err != nil {
		log.Errorf("Failed to apply TLS security profile: %v", err)
	} else if !applied {
		log.Warn("No TLS security profile settings found; relying on Go defaults")
	}

	return tlsConfig
}

func mockClientTLSConfig() *tls.Config {
	return buildMockClientTLSConfig(collectorCertPath)
}

// buildMockClientTLSConfig returns a client TLS config (skipping verification, TLS 1.3) when the given
// cert file exists, or nil otherwise. The path is a parameter so this can be unit-tested.
// This is used only by the mock client (--mock); the collector server always supports TLS 1.3, so it
// stays pinned to 1.3 regardless of the cluster profile applied to the real server.
func buildMockClientTLSConfig(certPath string) *tls.Config {
	if _, err := os.Stat(certPath); err != nil {
		return nil
	}
	return &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS13,
	}
}
