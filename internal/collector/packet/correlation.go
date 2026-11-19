package packet

import (
	"time"

	"github.com/netobserv/flowlogs-pipeline/pkg/config"
	collectorformat "github.com/netobserv/network-observability-cli/internal/collector/format"
)

func (b *WireBuffer) bestWireMatchWithAmbiguityLocked(pt *pendingPlaintext) (int, int, bool) {
	b.preparePlaintextForMatchLocked(pt)
	bestIdx := -1
	bestScore := -1
	secondBest := -1
	var connection flowTuple
	hasConnection, multipleConnections := false, false
	for i, pkt := range b.packets {
		score := scorePlaintextWireMatch(pkt, pt, b.filters)
		// A consumed packet still proves that another connection was present.
		// Payload/time bonuses distinguish packets, not missing client ports.
		if score >= 0 {
			if tuple, ok := flowTupleFromMap(pkt.genericMap); ok {
				if hasConnection && !tuplesEqual(connection, tuple) && !tuplesReverseEqual(connection, tuple) {
					multipleConnections = true
				}
				connection, hasConnection = tuple, true
			}
		}
		if pkt.annotated {
			continue
		}
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
	if !plaintextHasTuple(pt.original) && multipleConnections {
		pt.connectionAmbiguous = true
	}
	return bestIdx, bestScore, ambiguous || pt.connectionAmbiguous
}

func minAnnotationScore(m config.GenericMap, filters Filters) int {
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

func (b *WireBuffer) preparePlaintextForMatchLocked(pt *pendingPlaintext) {
	if pt == nil {
		return
	}
	enrichPlaintextFromCaptureFilters(&pt.data, b.filters)
}

func (b *WireBuffer) matchPendingAgainstWireLocked(final bool) {
	remaining := b.pendingPlaintext[:0]
	for _, pt := range b.pendingPlaintext {
		if b.tryAnnotateLocked(pt, final) {
			b.finalizePlaintextLocked(pt)
			continue
		}
		remaining = append(remaining, pt)
	}
	b.pendingPlaintext = remaining
}

func (b *WireBuffer) tryAnnotateLocked(pt *pendingPlaintext, final bool) bool {
	bestIdx, bestScore, ambiguous := b.bestWireMatchWithAmbiguityLocked(pt)
	if bestIdx < 0 || ambiguous || bestScore < minAnnotationScore(pt.data, b.filters) {
		return false
	}

	pkt := b.packets[bestIdx]
	if pkt.annotated {
		return false
	}
	// With a missing endpoint, the first arriving packet is not proof of a
	// unique connection. Wait for the export window before inferring a tuple.
	if !final && !plaintextHasTuple(pt.original) &&
		time.Since(pt.receivedAt) < b.window && time.Since(pkt.receivedAt) < b.window {
		return false
	}
	overlayCorrelatedWireToPlaintext(&pt.data, pkt.genericMap)
	collectorformat.EnrichPlaintextForExport(&pt.data)
	pkt.annotated = true
	pkt.annotations = []string{plaintextAnnotationComment(pt.data, pt.id)}
	idCopy := pt.id
	pkt.packetID = &idCopy
	pt.data["PcapAnnotated"] = true
	return true
}

func (b *WireBuffer) finalizePlaintextLocked(pt *pendingPlaintext) {
	if b.finalizePlaintext != nil {
		b.finalizePlaintext(pt.data)
	}
}

// finalizeUnmatchedLocked exports a record that expired without a confirmed
// correlation, using the pristine copy so no speculatively-borrowed wire fields
// leak into an uncorrelated (PcapAnnotated=false) record.
func (b *WireBuffer) finalizeUnmatchedLocked(pt *pendingPlaintext) {
	rec := pt.original
	if rec == nil {
		rec = pt.data
	}
	// Preserve network direction for verified events even without an annotation.
	if rec["TupleSource"] == "kernel" {
		prepareUnmatchedPlaintext(&rec)
	}
	rec["PcapAnnotated"] = false
	if b.finalizePlaintext != nil {
		b.finalizePlaintext(rec)
	}
}

func (b *WireBuffer) flushAll() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.matchPendingAgainstWireLocked(true)
	for _, pkt := range b.packets {
		b.writeBufferedWirePacket(pkt)
	}
	b.packets = nil
	for _, pt := range b.pendingPlaintext {
		b.finalizeUnmatchedLocked(pt)
	}
	b.pendingPlaintext = nil
}

func (b *WireBuffer) flushExpiredLocked(now time.Time) {
	if len(b.packets) > 0 {
		b.matchPendingAgainstWireLocked(false)
		remaining := b.packets[:0]
		for _, pkt := range b.packets {
			if now.Sub(pkt.receivedAt) >= b.window {
				b.writeBufferedWirePacket(pkt)
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

func (b *WireBuffer) writeBufferedWirePacket(pkt *bufferedWirePacket) {
	if b.writePacket == nil || pkt == nil {
		return
	}
	_ = b.writePacket(&pkt.genericMap, pkt.data, pkt.annotations, pkt.packetID)
}
