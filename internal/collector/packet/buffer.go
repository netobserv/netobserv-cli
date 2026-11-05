package packet

import (
	"encoding/base64"
	"sync"
	"time"

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
	// original preserves the agent record before capture-filter hints are added.
	// Unmatched records are exported without inferred endpoints or metadata.
	original config.GenericMap
	id       uint64
	// Once competing connections are observed, expiry of one candidate must
	// not make a remaining connection appear uniquely attributable.
	connectionAmbiguous bool
}

type WireBuffer struct {
	mu                sync.Mutex
	window            time.Duration
	filters           Filters
	writePacket       func(*config.GenericMap, []byte, []string, *uint64) error
	packets           []*bufferedWirePacket
	pendingPlaintext  []*pendingPlaintext
	finalizePlaintext func(config.GenericMap)
	stopCh            chan struct{}
}

func NewWireBuffer(window time.Duration, finalize func(config.GenericMap), filters Filters, writePacket func(*config.GenericMap, []byte, []string, *uint64) error) *WireBuffer {
	if window <= 0 {
		window = plaintextCorrelationWindow
	}
	b := &WireBuffer{
		window:            window,
		filters:           filters,
		writePacket:       writePacket,
		finalizePlaintext: finalize,
		stopCh:            make(chan struct{}),
	}
	go b.flushLoop()
	return b
}

func (b *WireBuffer) Close() {
	close(b.stopCh)
	b.flushAll()
}

func (b *WireBuffer) flushLoop() {
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

func (b *WireBuffer) Enqueue(genericMap config.GenericMap, dataB64 string) error {
	data, err := base64.StdEncoding.DecodeString(dataB64)
	if err != nil {
		return err
	}
	now := time.Now()
	pkt := &bufferedWirePacket{
		receivedAt: now,
		packetTime: plaintextTimestamp(genericMap),
		genericMap: genericMap.Copy(),
		data:       data,
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.flushExpiredLocked(now)
	b.packets = append(b.packets, pkt)
	if len(b.packets) > maxBufferedWirePackets {
		b.writeBufferedWirePacket(b.packets[0])
		b.packets = b.packets[1:]
	}
	b.matchPendingAgainstWireLocked(false)
	return nil
}

// HandlePlaintext tries immediate and deferred matching against buffered wire packets.
func (b *WireBuffer) HandlePlaintext(m config.GenericMap, id uint64) {
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
	if b.tryAnnotateLocked(pt, false) {
		b.finalizePlaintextLocked(pt)
		return
	}
	b.pendingPlaintext = append(b.pendingPlaintext, pt)
}
