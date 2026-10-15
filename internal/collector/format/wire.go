package format

import (
	"bytes"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

// TCPPayloadInfo returns the TCP application payload length and whether the frame
// contains a parseable TCP layer.
func TCPPayloadInfo(frame []byte) (payloadLen int, hasTCP bool) {
	if len(frame) == 0 {
		return 0, false
	}
	for _, layerType := range []gopacket.LayerType{
		layers.LayerTypeEthernet,
		layers.LayerTypeLinuxSLL,
		layers.LayerTypeIPv4,
		layers.LayerTypeIPv6,
	} {
		packet := gopacket.NewPacket(frame, layerType, gopacket.Default)
		if packet == nil {
			continue
		}
		tcpLayer := packet.Layer(layers.LayerTypeTCP)
		if tcpLayer == nil {
			continue
		}
		tcp, ok := tcpLayer.(*layers.TCP)
		if !ok {
			return 0, true
		}
		return len(tcp.Payload), true
	}
	return 0, false
}

// ExtractTCPPayloadFromFrame returns the TCP application payload from a captured L2 frame.
func ExtractTCPPayloadFromFrame(frame []byte) []byte {
	payloadLen, hasTCP := TCPPayloadInfo(frame)
	if !hasTCP || payloadLen == 0 {
		return nil
	}
	for _, layerType := range []gopacket.LayerType{
		layers.LayerTypeEthernet,
		layers.LayerTypeLinuxSLL,
		layers.LayerTypeIPv4,
		layers.LayerTypeIPv6,
	} {
		packet := gopacket.NewPacket(frame, layerType, gopacket.Default)
		if packet == nil {
			continue
		}
		tcpLayer := packet.Layer(layers.LayerTypeTCP)
		if tcpLayer == nil {
			continue
		}
		tcp, ok := tcpLayer.(*layers.TCP)
		if !ok || len(tcp.Payload) == 0 {
			return nil
		}
		return append([]byte(nil), tcp.Payload...)
	}
	return nil
}

func looksLikeCleartextHTTP(data []byte) bool {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) < 9 {
		return false
	}
	switch {
	case bytes.HasPrefix(trimmed, []byte("HTTP/1.")),
		bytes.HasPrefix(trimmed, []byte("GET ")),
		bytes.HasPrefix(trimmed, []byte("POST ")),
		bytes.HasPrefix(trimmed, []byte("PUT ")),
		bytes.HasPrefix(trimmed, []byte("HEAD ")),
		bytes.HasPrefix(trimmed, []byte("DELETE ")),
		bytes.HasPrefix(trimmed, []byte("PATCH ")),
		bytes.HasPrefix(trimmed, []byte("OPTIONS ")),
		bytes.HasPrefix(trimmed, []byte("CONNECT ")),
		bytes.HasPrefix(trimmed, []byte("TRACE ")):
		return true
	default:
		return false
	}
}

// WireHTTPPayload renders cleartext HTTP from a PCA wire packet (full L2 frame).
func WireHTTPPayload(fullFrame []byte) string {
	app := ExtractTCPPayloadFromFrame(fullFrame)
	if len(app) == 0 && looksLikeCleartextHTTP(fullFrame) {
		app = fullFrame
	}
	if len(app) == 0 || !looksLikeCleartextHTTP(app) {
		return ""
	}
	display := PlaintextDisplayString(app)
	if isGarbageDisplay(display) {
		return ""
	}
	return ellipsizePlaintextDisplay(display, 4096)
}

// WirePayloadPanelTitle returns the display title for a captured wire payload.
func WirePayloadPanelTitle(data []byte) string {
	if WireHTTPPayload(data) != "" {
		return "Wire HTTP (cleartext)"
	}
	return "Wire payload"
}
