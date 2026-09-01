package cmd

import (
	"crypto/tls"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	grpcCollector "github.com/netobserv/flowlogs-pipeline/pkg/pipeline/write/grpc"
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
		MinVersion:   tls.VersionTLS13,
	}

	log.Info("TLS enabled for collector")
	return []grpcCollector.CollectorOption{
		grpcCollector.WithGRPCServerOptions(grpc.Creds(credentials.NewTLS(tlsConfig))),
	}
}

func mockClientTLSConfig() *tls.Config {
	return buildMockClientTLSConfig(collectorCertPath)
}

// buildMockClientTLSConfig returns a client TLS config (skipping verification, TLS 1.3) when the given
// cert file exists, or nil otherwise. The path is a parameter so this can be unit-tested.
func buildMockClientTLSConfig(certPath string) *tls.Config {
	if _, err := os.Stat(certPath); err != nil {
		return nil
	}
	return &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS13,
	}
}
