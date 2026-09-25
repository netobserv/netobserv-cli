package cmd

import (
	"net"
	"testing"

	"github.com/netobserv/flowlogs-pipeline/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestOptionValue(t *testing.T) {
	defer func() { options = "" }()
	options = "port=8443|peer_ip=10.244.2.2|enable_openssl=true"
	assert.Equal(t, "8443", optionValue("port"))
	assert.Equal(t, "10.244.2.2", optionValue("peer_ip"))
	assert.Empty(t, optionValue("missing"))
}

func TestParseCaptureFilters(t *testing.T) {
	defer func() { options = "" }()
	options = "port=8443|peer_ip=10.244.2.2"
	f := parseCaptureFilters()
	assert.Equal(t, []uint16{8443}, f.ports)
	assert.Len(t, f.peerIPs, 1)
	assert.Equal(t, "10.244.2.2", f.peerIPs[0].String())

	options = "peer_cidr=10.244.2.0/24"
	f = parseCaptureFilters()
	assert.Len(t, f.peerNets, 1)
	assert.True(t, f.peerNets[0].Contains(net.ParseIP("10.244.2.2")))
}

func TestWirePacketMatchesCaptureFilters(t *testing.T) {
	wire := config.GenericMap{
		"SrcAddr": "10.244.1.9", "DstAddr": "10.244.2.2",
		"SrcPort": "52442", "DstPort": "8443",
	}
	f := captureFilters{ports: []uint16{8443}}
	assert.True(t, wirePacketMatchesCaptureFilters(wire, f))

	f = captureFilters{peerIPs: []net.IP{net.ParseIP("10.244.2.2")}}
	assert.True(t, wirePacketMatchesCaptureFilters(wire, f))

	f = captureFilters{ports: []uint16{443}}
	assert.False(t, wirePacketMatchesCaptureFilters(wire, f))
}
