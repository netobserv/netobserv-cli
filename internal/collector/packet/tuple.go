package packet

import (
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/netobserv/flowlogs-pipeline/pkg/config"
)

func mapTimestamp(m config.GenericMap) time.Time {
	if t, ok := m["TimeFlowStartMs"].(float64); ok && t > 0 {
		return time.UnixMilli(int64(t))
	}
	if t, ok := m["Time"].(float64); ok {
		return time.Unix(int64(t), 0)
	}
	return time.Now()
}

func plaintextHasTuple(m config.GenericMap) bool {
	_, srcOK := validIPString(m["SrcAddr"])
	_, dstOK := validIPString(m["DstAddr"])
	_, hasSrcPort := mapPortFromGeneric(m, "SrcPort")
	_, hasDstPort := mapPortFromGeneric(m, "DstPort")
	return srcOK && dstOK && hasSrcPort && hasDstPort
}

func setPlaintextTuple(m *config.GenericMap, srcIP string, srcPort uint16, dstIP string, dstPort uint16) {
	if m == nil {
		return
	}
	(*m)["SrcAddr"] = srcIP
	(*m)["DstAddr"] = dstIP
	(*m)["SrcPort"] = srcPort
	(*m)["DstPort"] = dstPort
	(*m)["Proto"] = float64(6)
}

// overlayCorrelatedWireToPlaintext applies the wire 5-tuple and FLP Kubernetes fields
// after a successful plaintext ↔ wire correlation. Wire packets are enriched by the
// collector pipeline; plaintext records are not unless they carry usable Src/Dst IPs.
func overlayCorrelatedWireToPlaintext(pt *config.GenericMap, wire config.GenericMap) {
	if pt == nil {
		return
	}
	overlayWireTupleToPlaintext(pt, wire)
	// Existing endpoint metadata may describe the agent's local-first tuple or
	// an earlier candidate. Replace it together with the confirmed wire tuple,
	// including removing fields that the matched packet does not provide.
	for key := range *pt {
		if strings.HasPrefix(key, "SrcK8S_") || strings.HasPrefix(key, "DstK8S_") {
			delete(*pt, key)
		}
	}
	copyWireK8sFieldsToPlaintext(pt, wire)
}

func overlayWireTupleToPlaintext(pt *config.GenericMap, wire config.GenericMap) {
	if pt == nil {
		return
	}
	t, ok := flowTupleFromMap(wire)
	if !ok {
		return
	}
	// The matched packet already travels in the plaintext direction. Reads
	// arrive at the local process; reversing them would report it as sender.
	setPlaintextTuple(pt, t.srcIP, t.srcPort, t.dstIP, t.dstPort)
}

func copyWireK8sFieldsToPlaintext(pt *config.GenericMap, wire config.GenericMap) {
	if pt == nil || wire == nil {
		return
	}
	for k, v := range wire {
		if !isWireK8sEnrichmentField(k) {
			continue
		}
		if hasMeaningfulGenericValue((*pt)[k]) {
			continue
		}
		(*pt)[k] = v
	}
}

func isWireK8sEnrichmentField(key string) bool {
	return strings.HasPrefix(key, "SrcK8S_") ||
		strings.HasPrefix(key, "DstK8S_") ||
		strings.HasPrefix(key, "K8S_")
}

func hasMeaningfulGenericValue(v interface{}) bool {
	if v == nil {
		return false
	}
	switch t := v.(type) {
	case string:
		return t != ""
	case float64:
		return t != 0
	case int:
		return t != 0
	case int64:
		return t != 0
	default:
		return true
	}
}

func flowTupleFromMap(m config.GenericMap) (flowTuple, bool) {
	src, ok1 := validIPString(m["SrcAddr"])
	dst, ok2 := validIPString(m["DstAddr"])
	if !ok1 || !ok2 {
		return flowTuple{}, false
	}
	srcPort, ok3 := mapPortFromGeneric(m, "SrcPort")
	dstPort, ok4 := mapPortFromGeneric(m, "DstPort")
	if !ok3 || !ok4 {
		return flowTuple{}, false
	}
	return flowTuple{srcIP: src, dstIP: dst, srcPort: srcPort, dstPort: dstPort}, true
}

func validIPString(v interface{}) (string, bool) {
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	if s == "" || s == "0.0.0.0" || s == "::" {
		return "", false
	}
	ip := net.ParseIP(s)
	if ip == nil || ip.IsUnspecified() {
		return "", false
	}
	return ip.String(), true
}

func mapPortFromGeneric(m config.GenericMap, key string) (uint16, bool) {
	switch v := m[key].(type) {
	case string:
		return parsePortString(v)
	case float64:
		return portInRange(int64(v))
	case uint16:
		return v, true
	case int:
		return portInRange(int64(v))
	case int64:
		return portInRange(v)
	default:
		return 0, false
	}
}

// portInRange rejects values outside the valid TCP/UDP port range instead of
// silently wrapping them to a uint16.
func portInRange(v int64) (uint16, bool) {
	if v < 0 || v > 65535 {
		return 0, false
	}
	return uint16(v), true
}

// parsePortString accepts gopacket-style values such as "443(https)".
func parsePortString(s string) (uint16, bool) {
	if i := strings.IndexByte(s, '('); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	p, err := strconv.ParseUint(s, 10, 16)
	if err != nil {
		return 0, false
	}
	return uint16(p), true
}

func tuplesEqual(a, b flowTuple) bool {
	return a.srcIP == b.srcIP && a.dstIP == b.dstIP && a.srcPort == b.srcPort && a.dstPort == b.dstPort
}

func tuplesReverseEqual(a, b flowTuple) bool {
	return a.srcIP == b.dstIP && a.dstIP == b.srcIP && a.srcPort == b.dstPort && a.dstPort == b.srcPort
}

func plaintextPeerIP(m config.GenericMap) (string, bool) {
	podIP, _, ok := plaintextPodEndpoint(m)
	if !ok {
		return "", false
	}
	for _, key := range []string{"DstAddr", "SrcAddr"} {
		if ip, valid := validIPString(m[key]); valid && ip != podIP {
			return ip, true
		}
	}
	return "", false
}

func plaintextPodEndpoint(m config.GenericMap) (ip string, port uint16, ok bool) {
	if src, valid := validIPString(m["SrcAddr"]); valid {
		if p, hasPort := mapPortFromGeneric(m, "SrcPort"); hasPort && p > 0 {
			return src, p, true
		}
	}
	if dst, valid := validIPString(m["DstAddr"]); valid {
		if p, hasPort := mapPortFromGeneric(m, "DstPort"); hasPort && p > 0 {
			return dst, p, true
		}
	}
	return "", 0, false
}

func wirePacketHasIP(m config.GenericMap, ip string) bool {
	src, srcOK := validIPString(m["SrcAddr"])
	dst, dstOK := validIPString(m["DstAddr"])
	return (srcOK && src == ip) || (dstOK && dst == ip)
}

func wirePacketHasIPPort(m config.GenericMap, ip string, port uint16) bool {
	src, srcOK := validIPString(m["SrcAddr"])
	dst, dstOK := validIPString(m["DstAddr"])
	sp, spOK := mapPortFromGeneric(m, "SrcPort")
	dp, dpOK := mapPortFromGeneric(m, "DstPort")
	if srcOK && src == ip && spOK && sp == port {
		return true
	}
	if dstOK && dst == ip && dpOK && dp == port {
		return true
	}
	return false
}

func wireEgressFrom(m config.GenericMap, ip string, port uint16) bool {
	src, srcOK := validIPString(m["SrcAddr"])
	sp, spOK := mapPortFromGeneric(m, "SrcPort")
	return srcOK && src == ip && spOK && sp == port
}

func wireIngressTo(m config.GenericMap, ip string, port uint16) bool {
	dst, dstOK := validIPString(m["DstAddr"])
	dp, dpOK := mapPortFromGeneric(m, "DstPort")
	return dstOK && dst == ip && dpOK && dp == port
}
