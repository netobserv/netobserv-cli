package flow

import (
	"testing"

	"github.com/netobserv/flowlogs-pipeline/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestBufferRetainsNewestFlowRecords(t *testing.T) {
	b := NewBuffer()
	for i := 0; i < defaultKeepCount+5; i++ {
		b.Append(config.GenericMap{"TimeFlowEndMs": float64(i)}, "Flow", false, false)
	}

	records := b.Snapshot("Flow", false)
	assert.Len(t, records, defaultKeepCount)
	assert.Equal(t, float64(5), records[0]["TimeFlowEndMs"])
	assert.Equal(t, float64(defaultKeepCount+4), records[len(records)-1]["TimeFlowEndMs"])
}

func TestBufferPacketSnapshotMergesWireAndPlaintextRecords(t *testing.T) {
	b := NewBuffer()
	b.Append(config.GenericMap{"Time": float64(3), "Data": "wire"}, "Packet", true, false)
	b.Append(config.GenericMap{"Time": float64(2), "RecordType": "plaintext"}, "Packet", true, false)

	records := b.Snapshot("Packet", true)
	assert.Len(t, records, 2)
	assert.Equal(t, float64(2), records[0]["Time"])
	assert.Equal(t, float64(3), records[1]["Time"])
}

func TestBufferResetAndClearPackets(t *testing.T) {
	b := NewBuffer()
	b.Append(config.GenericMap{"Time": float64(1), "Data": "wire"}, "Packet", true, false)
	b.Append(config.GenericMap{"Time": float64(2), "RecordType": "plaintext"}, "Packet", true, false)
	b.ClearPackets()
	assert.Empty(t, b.Snapshot("Packet", true))

	b.Append(config.GenericMap{"Time": float64(3)}, "Flow", false, false)
	b.Reset()
	assert.Empty(t, b.Snapshot("Flow", false))
}

func TestBufferIgnoresRecordsWhilePaused(t *testing.T) {
	b := NewBuffer()
	b.Append(config.GenericMap{"Time": float64(1)}, "Flow", false, true)
	assert.Empty(t, b.Snapshot("Flow", false))
}

func TestVisibleFiltersAndLimitsRecords(t *testing.T) {
	records := []config.GenericMap{
		{"Index": 1, "Name": "keep"},
		{"Index": 2, "Name": "drop"},
		{"Index": 3, "Name": "keep"},
	}
	visible := Visible(records, nil, []string{"keep"}, 1, false)
	assert.Len(t, visible, 1)
	assert.Equal(t, 3, visible[0]["Index"])
}

func TestVisibleKeepsSelectedFlowOutsideCurrentSnapshot(t *testing.T) {
	current := []config.GenericMap{{"Index": 3, "Name": "new"}}
	existing := []config.GenericMap{{"Index": 1, "Name": "selected"}}
	visible := Visible(current, existing, nil, 10, false)
	assert.Equal(t, []int{1, 3}, []int{visible[0]["Index"].(int), visible[1]["Index"].(int)})
}
