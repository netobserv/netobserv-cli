package packet

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/netobserv/flowlogs-pipeline/pkg/config"

	collectorformat "github.com/netobserv/network-observability-cli/internal/collector/format"
)

var plaintextPacketID uint64

const CorrelationWindow = 30 * time.Second

func nextPlaintextPacketID() uint64 {
	return atomic.AddUint64(&plaintextPacketID, 1)
}

func assignPlaintextPacketID(m *config.GenericMap) uint64 {
	id := nextPlaintextPacketID()
	(*m)["PacketID"] = id
	return id
}

func AssignPlaintextPacketID(m *config.GenericMap) uint64 { return assignPlaintextPacketID(m) }

func plaintextTimestamp(m config.GenericMap) time.Time {
	if t, ok := m["TimeFlowStartMs"].(float64); ok && t > 0 {
		return time.UnixMilli(int64(t))
	}
	if t, ok := m["Time"].(float64); ok && t > 0 {
		return time.Unix(int64(t), 0)
	}
	// Keep absence distinguishable from an actual event timestamp so receive
	// time is used only when the capture timestamp is unavailable.
	return time.Time{}
}

func plaintextFiveTupleLine(m config.GenericMap) string {
	src, _ := m["SrcAddr"].(string)
	dst, _ := m["DstAddr"].(string)
	if src == "" && dst == "" {
		return ""
	}
	srcPort, hasSrcPort := mapPortFromGeneric(m, "SrcPort")
	dstPort, hasDstPort := mapPortFromGeneric(m, "DstPort")
	if hasSrcPort && hasDstPort {
		return fmt.Sprintf("%s:%d -> %s:%d", src, srcPort, dst, dstPort)
	}
	if src != "" || dst != "" {
		return fmt.Sprintf("%s -> %s", src, dst)
	}
	return ""
}

func plaintextAnnotationComment(m config.GenericMap, id uint64) string {
	var b strings.Builder
	fmt.Fprintf(&b, "TLS Plaintext (PacketID: %d)\n", id)
	if dir, ok := m["Direction"].(string); ok && dir != "" {
		fmt.Fprintf(&b, "Direction: %s\n", dir)
	}
	if src, ok := m["TLSSource"].(string); ok && src != "" {
		fmt.Fprintf(&b, "TLSSource: %s\n", src)
	}
	if tuple := plaintextFiveTupleLine(m); tuple != "" {
		fmt.Fprintf(&b, "5-tuple: %s\n", tuple)
	}
	if pid, ok := m["Pid"]; ok {
		fmt.Fprintf(&b, "Pid: %v\n", pid)
	}
	if preview, ok := m["PlaintextPreview"].(string); ok && preview != "" {
		b.WriteString("---\n")
		if payload := collectorformat.PlaintextPayloadBytes(m); len(payload) > 0 {
			b.WriteString(collectorformat.PlaintextPayload(payload))
		} else {
			b.WriteString(preview)
		}
	}
	return b.String()
}

// Only a tuple captured from the socket during the TLS call proves connection
// identity. Older agents guessed from /proc after descriptors could be reused.
func plaintextTupleVerified(m config.GenericMap) bool {
	return m["TupleSource"] == "kernel" && plaintextHasTuple(m)
}

func PlaintextTupleVerified(m config.GenericMap) bool { return plaintextTupleVerified(m) }

func prepareUnmatchedPlaintext(m *config.GenericMap) {
	if !plaintextTupleVerified(*m) {
		for key := range *m {
			if key == "SrcAddr" || key == "DstAddr" || key == "SrcPort" || key == "DstPort" ||
				strings.HasPrefix(key, "SrcK8S_") || strings.HasPrefix(key, "DstK8S_") {
				delete(*m, key)
			}
		}
		return
	}
	if (*m)["Direction"] != "read" {
		return
	}
	// The agent exports local first; JSONL endpoints follow network direction
	// even when no wire packet could be annotated.
	original := m.Copy()
	for key := range original {
		if strings.HasPrefix(key, "Src") || strings.HasPrefix(key, "Dst") {
			delete(*m, key)
		}
	}
	for key, value := range original {
		if strings.HasPrefix(key, "Src") {
			(*m)["Dst"+key[3:]] = value
		}
		if strings.HasPrefix(key, "Dst") {
			(*m)["Src"+key[3:]] = value
		}
	}
}

func PrepareUnmatchedPlaintext(m *config.GenericMap) { prepareUnmatchedPlaintext(m) }
