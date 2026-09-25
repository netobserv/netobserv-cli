package cmd

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/netobserv/flowlogs-pipeline/pkg/config"
)

var plaintextPacketID uint64

func nextPlaintextPacketID() uint64 {
	return atomic.AddUint64(&plaintextPacketID, 1)
}

func assignPlaintextPacketID(m *config.GenericMap) uint64 {
	id := nextPlaintextPacketID()
	(*m)["PacketID"] = id
	return id
}

func plaintextTimestamp(m config.GenericMap) time.Time {
	if t, ok := m["TimeFlowStartMs"].(float64); ok && t > 0 {
		return time.UnixMilli(int64(t))
	}
	if t, ok := m["Time"].(float64); ok {
		return time.Unix(int64(t), 0)
	}
	return time.Now()
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
		if payload := plaintextPayloadBytes(m); len(payload) > 0 {
			b.WriteString(formatPlaintextPayload(payload))
		} else {
			b.WriteString(preview)
		}
	}
	return b.String()
}
