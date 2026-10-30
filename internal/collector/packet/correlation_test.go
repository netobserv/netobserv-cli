package packet

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/netobserv/flowlogs-pipeline/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestPlaintextCorrelationKeepsClientPort(t *testing.T) {
	for _, direction := range []string{"read", "write"} {
		for _, wireFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/wireFirst=%t", direction, wireFirst), func(t *testing.T) {
				var finalized []config.GenericMap
				buf := newWirePacketBuffer(nil, time.Minute, func(m config.GenericMap) {
					finalized = append(finalized, m.Copy())
				}, Filters{Ports: []uint16{8443}, PeerIPs: []net.IP{net.ParseIP("10.128.2.18")}})
				t.Cleanup(buf.Close)
				stamp := float64(time.Now().UnixMilli())
				pt := config.GenericMap{
					"TimeFlowStartMs": stamp, "Direction": direction, "PlaintextLen": float64(32),
					"SrcAddr": "10.128.2.18", "SrcPort": float64(8443),
					"DstAddr": "10.128.2.19", "DstPort": float64(50150),
				}
				enqueue := func(port uint16) {
					t.Helper()
					src, dst, sp, dp := "10.128.2.18", "10.128.2.19", uint16(8443), port
					if direction == "read" {
						src, dst, sp, dp = dst, src, dp, sp
					}
					wire := config.GenericMap{"TimeFlowStartMs": stamp, "SrcAddr": src, "DstAddr": dst, "SrcPort": sp, "DstPort": dp}
					frame := buildTestTCPFrameWithTuple(t, net.ParseIP(src), net.ParseIP(dst), sp, dp, bytes.Repeat([]byte("x"), 128))
					assert.NoError(t, buf.Enqueue(wire, base64.StdEncoding.EncodeToString(frame)))
				}
				if !wireFirst {
					buf.HandlePlaintext(pt, 1)
				}
				enqueue(50118)
				if wireFirst {
					buf.HandlePlaintext(pt, 1)
				}
				assert.Empty(t, finalized, "payload bonuses must not permit another connection's port")
				enqueue(50150)
				if assert.Len(t, finalized, 1) {
					key := "DstPort"
					if direction == "read" {
						key = "SrcPort"
					}
					assert.Equal(t, uint16(50150), finalized[0][key])
					assert.Equal(t, true, finalized[0]["PcapAnnotated"])
				}
			})
		}
	}
}

func TestPlaintextCorrelationDoesNotPromotePartialTuple(t *testing.T) {
	var finalized []config.GenericMap
	buf := newWirePacketBuffer(nil, time.Minute, func(m config.GenericMap) {
		finalized = append(finalized, m.Copy())
	}, Filters{})
	stamp := float64(time.Now().UnixMilli())
	// The candidate shares the server endpoint, but contradicts the known peer.
	wire := config.GenericMap{
		"TimeFlowStartMs": stamp, "SrcAddr": "10.128.2.18", "SrcPort": "8443",
		"DstAddr": "10.128.2.20", "DstPort": "50118",
	}
	assert.NoError(t, buf.Enqueue(wire, "Zm9v"))
	pt := config.GenericMap{
		"TimeFlowStartMs": stamp, "Direction": "write",
		"SrcAddr": "10.128.2.18", "SrcPort": float64(8443), "DstAddr": "10.128.2.19",
	}
	buf.HandlePlaintext(pt, 1)
	buf.Close()
	if assert.Len(t, finalized, 1) {
		assert.Equal(t, false, finalized[0]["PcapAnnotated"])
		assert.Equal(t, "10.128.2.19", finalized[0]["DstAddr"])
		assert.NotContains(t, finalized[0], "DstPort")
	}
}

func TestPlaintextCorrelationWaitsForCompetingConnections(t *testing.T) {
	for _, alreadyAnnotated := range []bool{false, true} {
		t.Run(fmt.Sprintf("alreadyAnnotated=%t", alreadyAnnotated), func(t *testing.T) {
			var finalized []config.GenericMap
			buf := newWirePacketBuffer(nil, time.Minute, func(m config.GenericMap) {
				finalized = append(finalized, m.Copy())
			}, Filters{})
			stamp := float64(time.Now().UnixMilli())
			pt := config.GenericMap{
				"TimeFlowStartMs": stamp, "Direction": "write", "PlaintextLen": float64(32),
				"SrcAddr": "10.128.2.18", "SrcPort": float64(8443),
			}
			buf.HandlePlaintext(pt, 1)
			for i, port := range []uint16{50118, 50150} {
				wire := config.GenericMap{
					"TimeFlowStartMs": stamp, "SrcAddr": "10.128.2.18", "SrcPort": "8443",
					"DstAddr": "10.128.2.19", "DstPort": port,
				}
				data := "Zm9v"
				if i == 1 {
					// Unequal payload scores cannot disambiguate different connections.
					frame := buildTestTCPFrameWithTuple(t, net.ParseIP("10.128.2.18"), net.ParseIP("10.128.2.19"), 8443, port, bytes.Repeat([]byte("x"), 128))
					data = base64.StdEncoding.EncodeToString(frame)
				}
				assert.NoError(t, buf.Enqueue(wire, data))
				assert.Empty(t, finalized, "partial records must await the correlation window")
				if i == 0 && alreadyAnnotated {
					buf.mu.Lock()
					buf.packets[0].annotated = true
					buf.mu.Unlock()
				}
			}
			// Simulate expiry of the first connection. Its earlier presence
			// still makes the missing client port ambiguous.
			buf.mu.Lock()
			buf.packets = buf.packets[1:]
			buf.mu.Unlock()
			buf.Close()
			if assert.Len(t, finalized, 1) {
				assert.Equal(t, false, finalized[0]["PcapAnnotated"])
				assert.NotContains(t, finalized[0], "DstPort")
			}
		})
	}
}

func TestPlaintextCorrelationCaptureTimeIsAuthoritative(t *testing.T) {
	now := time.Now()
	pkt := &bufferedWirePacket{receivedAt: now, packetTime: now}
	pt := &pendingPlaintext{receivedAt: now, eventTime: now}
	assert.True(t, timesCorrelated(pkt, pt))
	best := timeCorrelationBonus(pkt, pt)
	for _, delta := range []time.Duration{-3 * time.Second, 6 * time.Second} {
		pt.eventTime = now.Add(delta)
		assert.False(t, timesCorrelated(pkt, pt), "coincident export does not override capture time")
		assert.Zero(t, timeCorrelationBonus(pkt, pt))
	}
	for _, delta := range []time.Duration{-time.Second, time.Second} {
		pt.eventTime = now.Add(delta)
		assert.Less(t, timeCorrelationBonus(pkt, pt), best, "closest event must get the highest bonus")
	}
	pt.eventTime = time.Time{}
	assert.True(t, timesCorrelated(pkt, pt), "missing capture time permits receive-time fallback")
	pt.receivedAt = now.Add(plaintextCorrelationWindow + time.Second)
	assert.False(t, timesCorrelated(pkt, pt))
	assert.True(t, plaintextTimestamp(config.GenericMap{}).IsZero())
}

func TestCaptureFiltersPreserveKnownPlaintextEndpoint(t *testing.T) {
	for _, direction := range []string{"read", "write"} {
		pt := config.GenericMap{
			"Direction": direction, "SrcAddr": "10.128.2.19", "SrcPort": float64(50150),
		}
		filters := Filters{Ports: []uint16{8443}, PeerIPs: []net.IP{net.ParseIP("10.128.2.18")}}
		enrichPlaintextFromCaptureFilters(&pt, filters)
		assert.Equal(t, "10.128.2.19", pt["SrcAddr"])
		assert.Equal(t, float64(50150), pt["SrcPort"])
		enrichPlaintextFromCaptureFilters(&pt, Filters{Ports: []uint16{8443}})
		assert.Equal(t, float64(50150), pt["SrcPort"])
		assert.NotContains(t, pt, "DstPort")

		pt = config.GenericMap{"Direction": direction, "DstPort": float64(50150)}
		enrichPlaintextFromCaptureFilters(&pt, Filters{Ports: []uint16{8443}})
		assert.Equal(t, float64(50150), pt["DstPort"])
	}
}

func TestPlaintextCorrelationCompletesPartialOnExpiry(t *testing.T) {
	var finalized []config.GenericMap
	buf := newWirePacketBuffer(nil, time.Minute, func(m config.GenericMap) {
		finalized = append(finalized, m.Copy())
	}, Filters{})
	t.Cleanup(buf.Close)
	stamp := float64(time.Now().UnixMilli())
	pt := config.GenericMap{
		"TimeFlowStartMs": stamp, "Direction": "write",
		"SrcAddr": "10.128.2.18", "SrcPort": float64(8443),
	}
	buf.HandlePlaintext(pt, 1)
	wire := pt.Copy()
	wire["DstAddr"], wire["DstPort"] = "10.128.2.19", float64(50150)
	assert.NoError(t, buf.Enqueue(wire, "Zm9v"))
	assert.Empty(t, finalized)
	buf.mu.Lock()
	buf.pendingPlaintext[0].receivedAt = time.Now().Add(-2 * time.Minute)
	buf.flushExpiredLocked(time.Now())
	buf.mu.Unlock()
	if assert.Len(t, finalized, 1) {
		assert.Equal(t, true, finalized[0]["PcapAnnotated"])
		assert.Equal(t, uint16(50150), finalized[0]["DstPort"])
	}
}
