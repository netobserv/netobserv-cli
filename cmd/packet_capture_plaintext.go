package cmd

import (
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
