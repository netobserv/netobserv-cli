package packet

import (
	"strings"
	"time"

	"github.com/netobserv/flowlogs-pipeline/pkg/config"
	collectorformat "github.com/netobserv/network-observability-cli/internal/collector/format"
)

func receiveTimeDelta(pkt *bufferedWirePacket, pt *pendingPlaintext) time.Duration {
	d := pt.receivedAt.Sub(pkt.receivedAt)
	if d < 0 {
		d = -d
	}
	return d
}

func timesCorrelated(pkt *bufferedWirePacket, pt *pendingPlaintext) bool {
	if !pkt.packetTime.IsZero() && !pt.eventTime.IsZero() {
		agentDelta := pt.eventTime.Sub(pkt.packetTime)
		return agentDelta >= plaintextMatchMinDelta && agentDelta <= plaintextMatchMaxSkew
	}
	return receiveTimeDelta(pkt, pt) <= plaintextCorrelationWindow
}

// timeCorrelationBonus returns a small proximity bonus in [0, scoreTimeBonusMax]:
// closer in time scores higher, but the bonus stays a tiebreaker rather than
// overriding the match-quality tier (see scoreTimeBonusMax).
func timeCorrelationBonus(pkt *bufferedWirePacket, pt *pendingPlaintext) int {
	if !pkt.packetTime.IsZero() && !pt.eventTime.IsZero() {
		delta := pt.eventTime.Sub(pkt.packetTime)
		limit := plaintextMatchMaxSkew
		if delta < 0 {
			delta = -delta
			limit = -plaintextMatchMinDelta
		}
		if delta <= limit {
			return int(scoreTimeBonusMax * (limit - delta).Milliseconds() / limit.Milliseconds())
		}
		return 0
	}
	recvDelta := receiveTimeDelta(pkt, pt)
	if recvDelta <= plaintextCorrelationWindow {
		span := plaintextCorrelationWindow.Milliseconds()
		return int(scoreTimeBonusMax * (plaintextCorrelationWindow - recvDelta).Milliseconds() / span)
	}
	return 0
}

func scorePlaintextWireMatch(pkt *bufferedWirePacket, pt *pendingPlaintext, filters Filters) int {
	if !timesCorrelated(pkt, pt) {
		return -1
	}
	if !plaintextWirePodCompatible(pkt, pt.data) {
		return -1
	}
	if !plaintextEndpointsCompatible(pkt.genericMap, pt.data) {
		return -1
	}
	// Direction is relative to the local TLS process, whose endpoint is kept
	// first while matching. A reverse-direction packet must never win through
	// timing/payload bonuses or a fallback to capture-filter matching.
	if podIP, port, ok := plaintextPodEndpoint(pt.data); ok {
		switch pt.data["Direction"] {
		case "write":
			if !wireEgressFrom(pkt.genericMap, podIP, port) {
				return -1
			}
		case "read":
			if !wireIngressTo(pkt.genericMap, podIP, port) {
				return -1
			}
		}
	}
	bonus := timeCorrelationBonus(pkt, pt)

	base := -1
	if plaintextHasTuple(pt.data) {
		// A supplied client port is a constraint, not a preference. Falling
		// back to a peer-IP match lets payload bonuses pass the strict threshold
		// and silently rewrites the record onto a different TCP connection.
		base = scoreStrictTupleMatch(pkt, pt.data)
	} else {
		if s := scoreStrictTupleMatch(pkt, pt.data); s >= 0 {
			base = s
		} else if s := scoreRemoteEndpointMatch(pkt, pt.data); s >= 0 {
			base = s
		} else if s := scoreLooseEndpointMatch(pkt, pt.data); s >= 0 {
			base = s
		} else if s := scoreCaptureFilterWireMatch(pkt, filters); s >= 0 {
			base = s
		}
	}
	if base < 0 {
		return -1
	}
	return applyWirePayloadScoreAdjustments(pkt, pt, base+bonus)
}

// Partial tuples may be completed, but known addresses and their ports must
// never be replaced by a looser endpoint or capture-filter match.
func plaintextEndpointsCompatible(wire, pt config.GenericMap) bool {
	for _, prefix := range []string{"Src", "Dst"} {
		ip, known := validIPString(pt[prefix+"Addr"])
		if !known {
			continue
		}
		wirePrefix := prefix
		if pt["Direction"] == "read" {
			if prefix == "Src" {
				wirePrefix = "Dst"
			} else {
				wirePrefix = "Src"
			}
		}
		wireIP, _ := validIPString(wire[wirePrefix+"Addr"])
		if wireIP != ip {
			return false
		}
		if port, ok := mapPortFromGeneric(pt, prefix+"Port"); ok && port > 0 {
			wirePort, ok := mapPortFromGeneric(wire, wirePrefix+"Port")
			if !ok || wirePort != port {
				return false
			}
		}
	}
	return true
}

func applyWirePayloadScoreAdjustments(pkt *bufferedWirePacket, pt *pendingPlaintext, score int) int {
	if pkt == nil {
		return score
	}
	tcpLen, hasTCP := collectorformat.TCPPayloadInfo(pkt.data)
	if !hasTCP {
		return score
	}
	if tcpLen == 0 {
		return -1
	}
	score += scorePayloadBonus
	if plainLen := plaintextWirePayloadLen(pt); plainLen > 0 && tcpLen >= plainLen {
		score += scorePayloadSizeBonus
	}
	return score
}

func plaintextWirePayloadLen(pt *pendingPlaintext) int {
	if pt == nil {
		return 0
	}
	if n, ok := pt.data["PlaintextLen"].(float64); ok && n > 0 {
		return int(n)
	}
	return len(collectorformat.PlaintextPayloadBytes(pt.data))
}

// plaintextWirePodCompatible rejects plaintext whose TLSSource does not match the
// workload hinted by Kubernetes metadata on the correlated wire packet.
func plaintextWirePodCompatible(pkt *bufferedWirePacket, pt config.GenericMap) bool {
	if pkt == nil {
		return true
	}
	tlsSource, _ := pt["TLSSource"].(string)
	if tlsSource == "" {
		return true
	}
	podName, podOwner := wirePodIdentity(pkt.genericMap)
	if podName == "" && podOwner == "" {
		return true
	}
	identity := strings.ToLower(podName + " " + podOwner)
	switch {
	case strings.Contains(identity, "openssl"):
		return tlsSource == "openssl" || tlsSource == "ktls"
	case strings.Contains(identity, "gotls"):
		return tlsSource == "gotls"
	case strings.Contains(identity, "ktls"):
		return tlsSource == "ktls" || tlsSource == "openssl"
	default:
		return true
	}
}

func wirePodIdentity(m config.GenericMap) (name string, owner string) {
	if kind, _ := m["DstK8S_Type"].(string); kind == "Pod" {
		if n, _ := m["DstK8S_Name"].(string); n != "" {
			name = n
		}
		if o, _ := m["DstK8S_OwnerName"].(string); o != "" {
			owner = o
		}
		if name != "" || owner != "" {
			return name, owner
		}
	}
	if kind, _ := m["SrcK8S_Type"].(string); kind == "Pod" {
		if n, _ := m["SrcK8S_Name"].(string); n != "" {
			name = n
		}
		if o, _ := m["SrcK8S_OwnerName"].(string); o != "" {
			owner = o
		}
	}
	return name, owner
}

func scoreCaptureFilterWireMatch(pkt *bufferedWirePacket, filters Filters) int {
	if !filters.active() || !wirePacketMatchesCaptureFilters(pkt.genericMap, filters) {
		return -1
	}
	score := scoreFilterMatchBase
	if len(filters.Ports) > 0 {
		score++
	}
	if len(filters.PeerIPs) > 0 || len(filters.PeerNets) > 0 {
		score++
	}
	return score
}

func scoreStrictTupleMatch(pkt *bufferedWirePacket, pt config.GenericMap) int {
	ptTuple, ok := flowTupleFromMap(pt)
	if !ok {
		return -1
	}
	wireTuple, ok := flowTupleFromMap(pkt.genericMap)
	if !ok {
		return -1
	}
	if !tuplesEqual(wireTuple, ptTuple) && !tuplesReverseEqual(wireTuple, ptTuple) {
		return -1
	}

	score := scoreStrictMatchBase
	if dir, ok := pt["Direction"].(string); ok {
		if dir == "write" && tuplesEqual(wireTuple, ptTuple) {
			score += 5
		}
		if dir == "read" && tuplesReverseEqual(wireTuple, ptTuple) {
			score += 5
		}
	}
	return score
}

func scoreRemoteEndpointMatch(pkt *bufferedWirePacket, pt config.GenericMap) int {
	podIP, port, ok := plaintextPodEndpoint(pt)
	if !ok {
		return -1
	}
	peerIP, ok := plaintextPeerIP(pt)
	if !ok || peerIP == podIP {
		return -1
	}
	if !wirePacketHasIPPort(pkt.genericMap, podIP, port) {
		return -1
	}
	if !wirePacketHasIP(pkt.genericMap, peerIP) {
		return -1
	}

	score := scoreRemoteMatchBase
	if dir, ok := pt["Direction"].(string); ok {
		if dir == "write" && wireEgressFrom(pkt.genericMap, podIP, port) && wirePacketHasIP(pkt.genericMap, peerIP) {
			score += 5
		}
		if dir == "read" && wireIngressTo(pkt.genericMap, podIP, port) && wirePacketHasIP(pkt.genericMap, peerIP) {
			score += 5
		}
	}
	return score
}

func scoreLooseEndpointMatch(pkt *bufferedWirePacket, pt config.GenericMap) int {
	podIP, port, ok := plaintextPodEndpoint(pt)
	if !ok {
		return -1
	}
	if !wirePacketHasIPPort(pkt.genericMap, podIP, port) {
		return -1
	}

	score := scoreLooseMatchBase
	if dir, ok := pt["Direction"].(string); ok {
		if dir == "write" && wireEgressFrom(pkt.genericMap, podIP, port) {
			score += 5
		}
		if dir == "read" && wireIngressTo(pkt.genericMap, podIP, port) {
			score += 5
		}
	}
	return score
}
