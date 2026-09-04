package cmd

import (
	"testing"

	"github.com/netobserv/flowlogs-pipeline/pkg/config"
)

func TestAssignPlaintextPacketID(t *testing.T) {
	plaintextPacketID = 0
	m1 := config.GenericMap{"RecordType": "plaintext"}
	id1 := assignPlaintextPacketID(&m1)
	m2 := config.GenericMap{"RecordType": "plaintext"}
	id2 := assignPlaintextPacketID(&m2)
	if id1 != 1 || id2 != 2 {
		t.Fatalf("expected sequential ids 1,2 got %d,%d", id1, id2)
	}
	if m1["PacketID"] != uint64(1) || m2["PacketID"] != uint64(2) {
		t.Fatalf("unexpected PacketID on maps: %v %v", m1["PacketID"], m2["PacketID"])
	}
}

func TestPlaintextTimestampUsesMillis(t *testing.T) {
	ts := plaintextTimestamp(config.GenericMap{"TimeFlowStartMs": float64(1_700_000_000_123)})
	if ts.UnixMilli() != 1_700_000_000_123 {
		t.Fatalf("unexpected timestamp %v", ts)
	}
}
