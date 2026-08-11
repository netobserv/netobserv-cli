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
	if _, err := os.Stat(collectorCertPath); err != nil {
		return nil
	}
	if _, err := os.Stat(collectorKeyPath); err != nil {
		return nil
	}

	cert, err := tls.LoadX509KeyPair(collectorCertPath, collectorKeyPath)
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
	if _, err := os.Stat(collectorCertPath); err != nil {
		return nil
	}
	return &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS13,
	}
}
