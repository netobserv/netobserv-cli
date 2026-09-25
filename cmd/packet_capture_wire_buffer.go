package cmd

import (
	"encoding/base64"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/netobserv/flowlogs-pipeline/pkg/config"
)

const (
	// Wire and plaintext records traverse separate agent export paths and can
	// arrive at the CLI seconds apart even for the same TLS event.
	plaintextCorrelationWindow = 30 * time.Second
	plaintextMatchMaxSkew      = 5 * time.Second
	plaintextMatchMinDelta     = -2 * time.Second
	maxBufferedWirePackets     = 20000

	scoreStrictMatchBase  = 100
	scoreRemoteMatchBase  = 75
	scoreLooseMatchBase   = 50
	scoreFilterMatchBase  = 50
	scoreAmbiguityMargin  = 5
	scorePayloadBonus     = 20
	scorePayloadSizeBonus = 5

	// scoreTimeBonusMax caps the time-proximity bonus so it only breaks ties
	// between same-tier candidates (its span exceeds scoreAmbiguityMargin) and
	// can never let a lower match tier outrank a higher one (its max is well
	// below the ~25-point gap between tuple/remote/loose tiers).
	scoreTimeBonusMax = 10
)

type flowTuple struct {
	srcIP   string
	dstIP   string
	srcPort uint16
	dstPort uint16
}

type bufferedWirePacket struct {
	receivedAt  time.Time
	packetTime  time.Time
	genericMap  config.GenericMap
	data        []byte
	annotations []string
	packetID    *uint64
	annotated   bool
}

type pendingPlaintext struct {
	receivedAt time.Time
	eventTime  time.Time
	data       config.GenericMap
	// original is the pristine record as received, before any speculative
	// enrichment borrowed from a candidate wire packet. It is exported if the
	// record expires without a confirmed correlation, so an uncorrelated record
	// never ships another flow's 5-tuple/peer IP/K8s metadata.
	original config.GenericMap
	id       uint64
}

type wirePacketBuffer struct {
	mu                sync.Mutex
	window            time.Duration
	ngw               *pcapgo.NgWriter
	filters           captureFilters
	packets           []*bufferedWirePacket
	pendingPlaintext  []*pendingPlaintext
	finalizePlaintext func(config.GenericMap)
	stopCh            chan struct{}
}

func newWirePacketBuffer(ngw *pcapgo.NgWriter, window time.Duration, finalize func(config.GenericMap), filters captureFilters) *wirePacketBuffer {
	if window <= 0 {
		window = plaintextCorrelationWindow
	}
	b := &wirePacketBuffer{
		window:            window,
		ngw:               ngw,
		filters:           filters,
		finalizePlaintext: finalize,
		stopCh:            make(chan struct{}),
	}
	go b.flushLoop()
	return b
}

func (b *wirePacketBuffer) Close() {
	close(b.stopCh)
	b.flushAll()
}

func (b *wirePacketBuffer) flushLoop() {
	ticker := time.NewTicker(b.window / 4)
	defer ticker.Stop()
	for {
		select {
		case <-b.stopCh:
			return
		case <-ticker.C:
			b.mu.Lock()
			b.flushExpiredLocked(time.Now())
			b.mu.Unlock()
		}
	}
}

func (b *wirePacketBuffer) Enqueue(genericMap config.GenericMap, dataB64 string) error {
	data, err := base64.StdEncoding.DecodeString(dataB64)
	if err != nil {
		return err
	}
	now := time.Now()
	pkt := &bufferedWirePacket{
		receivedAt: now,
		packetTime: mapTimestamp(genericMap),
		genericMap: genericMap.Copy(),
		data:       data,
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.flushExpiredLocked(now)
	b.packets = append(b.packets, pkt)
	if len(b.packets) > maxBufferedWirePackets {
		writeBufferedWirePacket(b.ngw, b.packets[0])
		b.packets = b.packets[1:]
	}
	b.matchPendingAgainstWireLocked()
	return nil
}

// HandlePlaintext tries immediate and deferred matching against buffered wire packets.
func (b *wirePacketBuffer) HandlePlaintext(m config.GenericMap, id uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	b.flushExpiredLocked(now)

	pt := &pendingPlaintext{
		receivedAt: now,
		eventTime:  plaintextTimestamp(m),
		data:       m.Copy(),
		id:         id,
	}
	pt.original = pt.data.Copy()
	b.enrichTupleFromWireLocked(pt)
	if b.tryAnnotateLocked(pt) {
		b.finalizePlaintextLocked(pt)
		return
	}
	b.pendingPlaintext = append(b.pendingPlaintext, pt)
}

func (b *wirePacketBuffer) bestWireMatchWithAmbiguityLocked(pt *pendingPlaintext) (int, int, bool) {
	b.preparePlaintextForMatchLocked(pt)
	bestIdx := -1
	bestScore := -1
	secondBest := -1
	for i, pkt := range b.packets {
		if pkt.annotated {
			continue
		}
		score := scorePlaintextWireMatch(pkt, pt, b.filters)
		if score > bestScore {
			secondBest = bestScore
			bestScore = score
			bestIdx = i
			continue
		}
		if score > secondBest {
			secondBest = score
		}
	}
	ambiguous := bestIdx >= 0 && secondBest >= 0 &&
		bestScore-secondBest <= scoreAmbiguityMargin
	return bestIdx, bestScore, ambiguous
}

func minAnnotationScore(m config.GenericMap, filters captureFilters) int {
	if plaintextHasTuple(m) {
		return scoreStrictMatchBase
	}
	podIP, port, ok := plaintextPodEndpoint(m)
	if ok && podIP != "" && port > 0 {
		if peer, peerOK := plaintextPeerIP(m); peerOK && peer != podIP {
			return scoreRemoteMatchBase
		}
		return scoreLooseMatchBase
	}
	if filters.active() {
		return scoreFilterMatchBase
	}
	return scoreLooseMatchBase
}

func (b *wirePacketBuffer) preparePlaintextForMatchLocked(pt *pendingPlaintext) {
	if pt == nil {
		return
	}
	enrichPlaintextFromCaptureFilters(&pt.data, b.filters)
}

func (b *wirePacketBuffer) enrichTupleFromWireLocked(pt *pendingPlaintext) {
	if plaintextHasTuple(pt.data) {
		return
	}
	bestIdx, bestScore, ambiguous := b.bestWireMatchWithAmbiguityLocked(pt)
	if bestIdx < 0 || ambiguous || bestScore < scoreLooseMatchBase {
		return
	}
	applyWireTupleToPlaintext(&pt.data, b.packets[bestIdx].genericMap)
	copyWireK8sFieldsToPlaintext(&pt.data, b.packets[bestIdx].genericMap)
	enrichPlaintextForExport(&pt.data)
}

func (b *wirePacketBuffer) matchPendingAgainstWireLocked() {
	remaining := b.pendingPlaintext[:0]
	for _, pt := range b.pendingPlaintext {
		b.enrichTupleFromWireLocked(pt)
		if b.tryAnnotateLocked(pt) {
			b.finalizePlaintextLocked(pt)
			continue
		}
		remaining = append(remaining, pt)
	}
	b.pendingPlaintext = remaining
}

func (b *wirePacketBuffer) tryAnnotateLocked(pt *pendingPlaintext) bool {
	bestIdx, bestScore, ambiguous := b.bestWireMatchWithAmbiguityLocked(pt)
	if bestIdx < 0 || ambiguous || bestScore < minAnnotationScore(pt.data, b.filters) {
		return false
	}

	pkt := b.packets[bestIdx]
	if pkt.annotated {
		return false
	}
	overlayCorrelatedWireToPlaintext(&pt.data, pkt.genericMap)
	enrichPlaintextForExport(&pt.data)
	pkt.annotated = true
	pkt.annotations = []string{plaintextAnnotationComment(pt.data, pt.id)}
	idCopy := pt.id
	pkt.packetID = &idCopy
	pt.data["PcapAnnotated"] = true
	return true
}

func (b *wirePacketBuffer) finalizePlaintextLocked(pt *pendingPlaintext) {
	if b.finalizePlaintext != nil {
		b.finalizePlaintext(pt.data)
	}
}

// finalizeUnmatchedLocked exports a record that expired without a confirmed
// correlation, using the pristine copy so no speculatively-borrowed wire fields
// leak into an uncorrelated (PcapAnnotated=false) record.
func (b *wirePacketBuffer) finalizeUnmatchedLocked(pt *pendingPlaintext) {
	rec := pt.original
	if rec == nil {
		rec = pt.data
	}
	rec["PcapAnnotated"] = false
	if b.finalizePlaintext != nil {
		b.finalizePlaintext(rec)
	}
}

func (b *wirePacketBuffer) flushAll() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.matchPendingAgainstWireLocked()
	for _, pkt := range b.packets {
		writeBufferedWirePacket(b.ngw, pkt)
	}
	b.packets = nil
	for _, pt := range b.pendingPlaintext {
		b.finalizeUnmatchedLocked(pt)
	}
	b.pendingPlaintext = nil
}

func (b *wirePacketBuffer) flushExpiredLocked(now time.Time) {
	if len(b.packets) > 0 {
		b.matchPendingAgainstWireLocked()
		remaining := b.packets[:0]
		for _, pkt := range b.packets {
			if now.Sub(pkt.receivedAt) >= b.window {
				writeBufferedWirePacket(b.ngw, pkt)
			} else {
				remaining = append(remaining, pkt)
			}
		}
		b.packets = remaining
	}

	if len(b.pendingPlaintext) == 0 {
		return
	}
	remainingPT := b.pendingPlaintext[:0]
	for _, pt := range b.pendingPlaintext {
		if now.Sub(pt.receivedAt) >= b.window {
			b.finalizeUnmatchedLocked(pt)
			continue
		}
		remainingPT = append(remainingPT, pt)
	}
	b.pendingPlaintext = remainingPT
}

func writeBufferedWirePacket(ngw *pcapgo.NgWriter, pkt *bufferedWirePacket) {
	if ngw == nil || pkt == nil {
		return
	}
	if err := writePacketDataWithOptions(ngw, &pkt.genericMap, pkt.data, pkt.annotations, pkt.packetID); err != nil {
		log.Error("failed to write buffered wire packet", err)
	}
}

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

// applyWireTupleToPlaintext borrows a candidate tuple for matching, keeping the
// local process endpoint in SrcAddr/SrcPort as in the incoming agent record.
// Export uses overlayWireTupleToPlaintext to restore wire direction.
func applyWireTupleToPlaintext(pt *config.GenericMap, wire config.GenericMap) {
	if pt == nil || plaintextHasTuple(*pt) {
		return
	}
	t, ok := flowTupleFromMap(wire)
	if !ok {
		return
	}
	dir, _ := (*pt)["Direction"].(string)
	if dir == "read" {
		setPlaintextTuple(pt, t.dstIP, t.dstPort, t.srcIP, t.srcPort)
		return
	}
	setPlaintextTuple(pt, t.srcIP, t.srcPort, t.dstIP, t.dstPort)
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

func receiveTimeDelta(pkt *bufferedWirePacket, pt *pendingPlaintext) time.Duration {
	d := pt.receivedAt.Sub(pkt.receivedAt)
	if d < 0 {
		d = -d
	}
	return d
}

func timesCorrelated(pkt *bufferedWirePacket, pt *pendingPlaintext) bool {
	agentDelta := pt.eventTime.Sub(pkt.packetTime)
	if agentDelta >= plaintextMatchMinDelta && agentDelta <= plaintextMatchMaxSkew {
		return true
	}
	return receiveTimeDelta(pkt, pt) <= plaintextCorrelationWindow
}

// timeCorrelationBonus returns a small proximity bonus in [0, scoreTimeBonusMax]:
// closer in time scores higher, but the bonus stays a tiebreaker rather than
// overriding the match-quality tier (see scoreTimeBonusMax).
func timeCorrelationBonus(pkt *bufferedWirePacket, pt *pendingPlaintext) int {
	agentDelta := pt.eventTime.Sub(pkt.packetTime)
	if agentDelta >= plaintextMatchMinDelta && agentDelta <= plaintextMatchMaxSkew {
		span := (plaintextMatchMaxSkew - plaintextMatchMinDelta).Milliseconds()
		return int(scoreTimeBonusMax * (plaintextMatchMaxSkew - agentDelta).Milliseconds() / span)
	}
	recvDelta := receiveTimeDelta(pkt, pt)
	if recvDelta <= plaintextCorrelationWindow {
		span := plaintextCorrelationWindow.Milliseconds()
		return int(scoreTimeBonusMax * (plaintextCorrelationWindow - recvDelta).Milliseconds() / span)
	}
	return 0
}

func scorePlaintextWireMatch(pkt *bufferedWirePacket, pt *pendingPlaintext, filters captureFilters) int {
	if pkt != nil && pkt.annotated {
		return -1
	}
	if !timesCorrelated(pkt, pt) {
		return -1
	}
	if !plaintextWirePodCompatible(pkt, pt.data) {
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
		if s := scoreStrictTupleMatch(pkt, pt.data); s >= 0 {
			base = s
		} else if s := scoreRemoteEndpointMatch(pkt, pt.data); s >= 0 {
			base = s
		}
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

func applyWirePayloadScoreAdjustments(pkt *bufferedWirePacket, pt *pendingPlaintext, score int) int {
	if pkt == nil {
		return score
	}
	tcpLen, hasTCP := wireTCPPayloadInfo(pkt.data)
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
	return len(plaintextPayloadBytes(pt.data))
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

func scoreCaptureFilterWireMatch(pkt *bufferedWirePacket, filters captureFilters) int {
	if !filters.active() || !wirePacketMatchesCaptureFilters(pkt.genericMap, filters) {
		return -1
	}
	score := scoreFilterMatchBase
	if len(filters.ports) > 0 {
		score++
	}
	if len(filters.peerIPs) > 0 || len(filters.peerNets) > 0 {
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

func writePacketDataWithOptions(
	ngw *pcapgo.NgWriter,
	genericMap *config.GenericMap,
	data []byte,
	extraComments []string,
	packetID *uint64,
) error {
	ts := mapTimestamp(*genericMap)

	keys := make([]string, 0, len(*genericMap))
	for k := range *genericMap {
		if k == "Time" || k == "Data" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	srcComment.WriteString("Source\n")
	dstComment.WriteString("Destination\n")
	commonComment.WriteString("Common\n")
	for _, k := range keys {
		id := toColID(k)
		str := fmt.Sprintf("%s: %v\n", toColName(id, 0), toColValue((*genericMap), id, 0))
		if strings.HasPrefix(k, "Src") {
			srcComment.WriteString(str)
		} else if strings.HasPrefix(k, "Dst") {
			dstComment.WriteString(str)
		} else {
			commonComment.WriteString(str)
		}
	}

	comments := []string{
		srcComment.String(),
		dstComment.String(),
		commonComment.String(),
	}
	comments = append(comments, extraComments...)

	opts := pcapgo.NgPacketOptions{Comments: comments}
	if packetID != nil {
		opts.PacketID = packetID
	}

	// ngwMu serializes this write against TLS keylog DSB writes (watchTLSKeylog)
	// and the SIGTERM flush (flushActivePacketWriter); NgWriter is not safe for
	// concurrent use.
	ngwMu.Lock()
	err := ngw.WritePacketWithOptions(gopacket.CaptureInfo{
		Timestamp:     ts,
		Length:        len(data),
		CaptureLength: len(data),
	}, data, opts)
	ngwMu.Unlock()

	srcComment.Reset()
	dstComment.Reset()
	commonComment.Reset()
	return err
}
