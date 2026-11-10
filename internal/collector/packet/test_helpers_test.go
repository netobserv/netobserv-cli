package packet

import (
	"net"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/netobserv/flowlogs-pipeline/pkg/config"
)

func newWirePacketBuffer(ngw *pcapgo.NgWriter, window time.Duration, finalize func(config.GenericMap), filters Filters) *WireBuffer {
	var writer func(*config.GenericMap, []byte, []string, *uint64) error
	if ngw != nil {
		writer = func(m *config.GenericMap, data []byte, comments []string, id *uint64) error {
			return ngw.WritePacketWithOptions(gopacket.CaptureInfo{Timestamp: mapTimestamp(*m), Length: len(data), CaptureLength: len(data)}, data, pcapgo.NgPacketOptions{Comments: comments, PacketID: id})
		}
	}
	return NewWireBuffer(window, finalize, filters, writer)
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
