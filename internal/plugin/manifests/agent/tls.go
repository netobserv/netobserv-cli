package agent

import (
	"fmt"

	flpapi "github.com/netobserv/flowlogs-pipeline/pkg/api"
	"github.com/netobserv/netobserv-operator/pkg/capture"
)

// EnableCollectorTLS replaces enableCollectorTLS: adds the CA volume/mount and
// collector-tls-config envFrom (the resolve-tls initContainer writes that ConfigMap from the
// cluster's TLS security profile, see internal/k8s/tls), and points the FLP pipeline's
// "send" (gRPC write) stage at the injected CA cert — all as typed fields instead of yq edits
// against a string that happens to contain JSON.
func (b *Builder) EnableCollectorTLS(caCertPath string) error {
	c := b.container()
	capture.ConfigureCollectorTLS(&b.ds.Spec.Template.Spec, c)

	send, err := b.pipelineStage("send")
	if err != nil {
		return err
	}
	if send.Write == nil || send.Write.GRPC == nil {
		return fmt.Errorf("FLP 'send' stage has no grpc write configuration")
	}
	send.Write.GRPC.TLS = &flpapi.ClientTLS{CACertPath: caCertPath}
	return nil
}
