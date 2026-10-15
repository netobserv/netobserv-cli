package flow

import (
	"slices"
	"sync"

	"github.com/netobserv/flowlogs-pipeline/pkg/config"
	collectorformat "github.com/netobserv/network-observability-cli/internal/collector/format"
	collectorpacket "github.com/netobserv/network-observability-cli/internal/collector/packet"
)

const (
	defaultKeepCount       = 100
	defaultWirePacketCount = 200
	defaultPlaintextCount  = 300
)

// Buffer owns captured flow records independently from terminal rendering.
type Buffer struct {
	mu          sync.RWMutex
	nextIndex   int
	flows       []config.GenericMap
	wirePackets []config.GenericMap
	plaintext   []config.GenericMap
}

func NewBuffer() *Buffer { return &Buffer{} }

func (b *Buffer) ClearPackets() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.wirePackets = nil
	b.plaintext = nil
}

func (b *Buffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextIndex = 0
	b.flows = nil
	b.wirePackets = nil
	b.plaintext = nil
}

func (b *Buffer) Append(record config.GenericMap, capture string, plaintextEnabled, paused bool) {
	if paused {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	record["Index"] = b.nextIndex
	b.nextIndex++

	if capture == "Packet" && plaintextEnabled {
		switch {
		case isPlaintext(record):
			appendSorted(&b.plaintext, record, defaultPlaintextCount, lessByPacket)
		case collectorpacket.IsWireRecord(record):
			appendSorted(&b.wirePackets, record, defaultWirePacketCount, lessByPacket)
		default:
			appendSorted(&b.flows, record, defaultKeepCount, lessByPacket)
		}
		return
	}

	b.flows = append(b.flows, record)
	sortByCapture(b.flows, capture)
	if len(b.flows) > defaultKeepCount {
		b.flows = b.flows[len(b.flows)-defaultKeepCount:]
	}
}

func (b *Buffer) Snapshot(capture string, plaintextEnabled bool) []config.GenericMap {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if capture == "Packet" && plaintextEnabled {
		merged := make([]config.GenericMap, 0, len(b.wirePackets)+len(b.plaintext))
		merged = append(merged, b.wirePackets...)
		merged = append(merged, b.plaintext...)
		collectorpacket.Sort(merged)
		return merged
	}
	return slices.Clone(b.flows)
}

func isPlaintext(record config.GenericMap) bool {
	return collectorformat.IsPlaintextRecord(record)
}

func appendSorted(records *[]config.GenericMap, record config.GenericMap, limit int, less func(config.GenericMap, config.GenericMap) bool) {
	*records = append(*records, record)
	slices.SortStableFunc(*records, func(a, b config.GenericMap) int {
		if less(a, b) {
			return -1
		}
		if less(b, a) {
			return 1
		}
		return 0
	})
	if len(*records) > limit {
		*records = (*records)[len(*records)-limit:]
	}
}

func lessByPacket(a, b config.GenericMap) bool { return collectorpacket.Less(a, b) }

func sortByCapture(records []config.GenericMap, capture string) {
	slices.SortStableFunc(records, func(a, b config.GenericMap) int {
		var less func(config.GenericMap, config.GenericMap) bool
		switch capture {
		case "Packet":
			less = collectorpacket.Less
		case "Flow":
			less = func(a, b config.GenericMap) bool {
				return collectorformat.ToFloat64(a, "TimeFlowEndMs") < collectorformat.ToFloat64(b, "TimeFlowEndMs")
			}
		default:
			less = func(a, b config.GenericMap) bool {
				return collectorformat.ToFloat64(a, "Time") < collectorformat.ToFloat64(b, "Time")
			}
		}
		if less(a, b) {
			return -1
		}
		if less(b, a) {
			return 1
		}
		return 0
	})
}
