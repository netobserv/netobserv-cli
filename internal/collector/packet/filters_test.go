package packet

import (
	"net"
	"testing"

	"github.com/netobserv/flowlogs-pipeline/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestOptionValue(t *testing.T) {
	options := "port=8443|peer_ip=10.244.2.2|enable_openssl=true"
	assert.Equal(t, "8443", optionValue(options, "port"))
	assert.Equal(t, "10.244.2.2", optionValue(options, "peer_ip"))
	assert.Empty(t, optionValue(options, "missing"))
}

func TestParseCaptureFilters(t *testing.T) {
	f := ParseFilters("port=8443|peer_ip=10.244.2.2")
	assert.Equal(t, []uint16{8443}, f.Ports)
	assert.Len(t, f.PeerIPs, 1)
	assert.Equal(t, "10.244.2.2", f.PeerIPs[0].String())

	f = ParseFilters("peer_cidr=10.244.2.0/24")
	assert.Len(t, f.PeerNets, 1)
	assert.True(t, f.PeerNets[0].Contains(net.ParseIP("10.244.2.2")))
}

func TestWirePacketMatchesCaptureFilters(t *testing.T) {
	wire := config.GenericMap{
		"SrcAddr": "10.244.1.9", "DstAddr": "10.244.2.2",
		"SrcPort": "52442", "DstPort": "8443",
	}
	f := Filters{Ports: []uint16{8443}}
	assert.True(t, wirePacketMatchesCaptureFilters(wire, f))

	f = Filters{PeerIPs: []net.IP{net.ParseIP("10.244.2.2")}}
	assert.True(t, wirePacketMatchesCaptureFilters(wire, f))

	f = Filters{Ports: []uint16{443}}
	assert.False(t, wirePacketMatchesCaptureFilters(wire, f))
}
