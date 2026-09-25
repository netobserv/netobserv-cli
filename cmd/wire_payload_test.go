package cmd

import (
	"net"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/assert"
)

func TestFormatWireHTTPPayloadFromFrame(t *testing.T) {
	httpBody := "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nX-NetObserv-Stack: http\r\n\r\nNETOBSERV-HTTP cleartext probe\n"
	frame := buildTestTCPFrame(t, []byte(httpBody))

	formatted := formatWireHTTPPayload(frame)
	assert.Contains(t, formatted, "HTTP/1.1 200 OK")
	assert.Contains(t, formatted, "NETOBSERV-HTTP cleartext probe")
}

func TestFormatWireHTTPPayloadNonHTTPReturnsEmpty(t *testing.T) {
	frame := buildTestTCPFrame(t, []byte{0x16, 0x03, 0x03, 0x00, 0x05, 0x01, 0x02, 0x03, 0x04})
	assert.Empty(t, formatWireHTTPPayload(frame))
}

func buildTestTCPFrame(t *testing.T, payload []byte) []byte {
	return buildTestTCPFrameWithTuple(t,
		net.IP{10, 0, 0, 1}, net.IP{10, 0, 0, 2}, 54321, 8080, payload)
}

func buildTestTCPFrameWithTuple(t *testing.T, srcIP, dstIP net.IP, srcPort, dstPort uint16, payload []byte) []byte {
	t.Helper()
	eth := &layers.Ethernet{
		SrcMAC:       net.HardwareAddr{0xde, 0xad, 0xbe, 0xef, 0x00, 0x01},
		DstMAC:       net.HardwareAddr{0xfe, 0xed, 0xfa, 0xce, 0x00, 0x02},
		EthernetType: layers.EthernetTypeIPv4,
	}
	ip := &layers.IPv4{
		Version:  4,
		TTL:      64,
		Protocol: layers.IPProtocolTCP,
		SrcIP:    srcIP,
		DstIP:    dstIP,
	}
	tcp := &layers.TCP{
		SrcPort: layers.TCPPort(srcPort),
		DstPort: layers.TCPPort(dstPort),
	}
	if err := tcp.SetNetworkLayerForChecksum(ip); err != nil {
		t.Fatal(err)
	}
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	layersToSerialize := []gopacket.SerializableLayer{eth, ip, tcp}
	if len(payload) > 0 {
		layersToSerialize = append(layersToSerialize, gopacket.Payload(payload))
	}
	if err := gopacket.SerializeLayers(buf, opts, layersToSerialize...); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
