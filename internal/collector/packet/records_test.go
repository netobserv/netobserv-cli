package packet

import (
	"bytes"
	"testing"

	"github.com/netobserv/flowlogs-pipeline/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestTrimFlowsToShowCountUnified(t *testing.T) {
	var flows []config.GenericMap
	for i := range 50 {
		flow := config.GenericMap{"Index": i, "Time": float64(i)}
		if i%5 == 0 {
			flow["RecordType"] = "plaintext"
			flow["PlaintextPreview"] = "GET /"
		} else {
			flow["Data"] = "d2lyZQ=="
		}
		flows = append(flows, flow)
	}

	shown := Trim(flows, 10)
	assert.Len(t, shown, 10)
	for _, flow := range shown {
		assert.GreaterOrEqual(t, int(flow["Time"].(float64)), 40)
	}
	plaintextCount := 0
	for _, flow := range shown {
		if IsPlaintextRecord(flow) {
			plaintextCount++
		}
	}
	assert.Equal(t, 2, plaintextCount)
}

func TestTrimPacketCaptureFlowsNewestByTime(t *testing.T) {
	var flows []config.GenericMap
	for i := range 40 {
		flows = append(flows, config.GenericMap{"Index": i, "Time": float64(i), "Data": "d2lyZQ=="})
	}
	flows = append(flows, config.GenericMap{
		"Index": 40, "Time": float64(10), "RecordType": "plaintext", "TLSSource": "gotls",
		"PlaintextPreview": "GET /health HTTP/1.1\r\n\r\n",
	})

	shown := Trim(flows, 5)
	assert.Len(t, shown, 5)
	for _, flow := range shown {
		assert.GreaterOrEqual(t, int(flow["Time"].(float64)), 35)
	}
}

func TestTrimPacketCaptureFlowsPlaintextOnly(t *testing.T) {
	var flows []config.GenericMap
	for i := range 30 {
		flows = append(flows, config.GenericMap{
			"Index": i, "Time": float64(i), "RecordType": "plaintext", "TLSSource": "openssl",
			"PlaintextPreview": "GET /health HTTP/1.1\r\n\r\n",
		})
	}

	shown := Trim(flows, 10)
	assert.Len(t, shown, 10)
	for _, flow := range shown {
		assert.True(t, IsPlaintextRecord(flow))
		assert.GreaterOrEqual(t, int(flow["Time"].(float64)), 20)
	}
}

func TestTrimPacketCaptureFlowsBinaryPlaintext(t *testing.T) {
	var flows []config.GenericMap
	for i := range 50 {
		flows = append(flows, config.GenericMap{
			"Index": i, "Time": float64(i), "RecordType": "plaintext", "TLSSource": "ktls",
			"Direction": "write", "PlaintextLen": float64(8190),
			"PlaintextPreview": string(bytes.Repeat([]byte{0x00, 0x01}, 128)),
		})
	}

	shown := Trim(flows, 10)
	assert.Len(t, shown, 10)
	for _, flow := range shown {
		assert.True(t, IsPlaintextRecord(flow))
	}
}

func TestTrimPacketCaptureFlowsWireAndPlaintextMix(t *testing.T) {
	var flows []config.GenericMap
	index := 0
	for i := range 80 {
		flows = append(flows, config.GenericMap{"Index": index, "Time": float64(i), "Data": "d2lyZQ=="})
		index++
	}
	for i := range 80 {
		flows = append(flows, config.GenericMap{
			"Index": index, "Time": float64(100 + i), "RecordType": "plaintext", "TLSSource": "ktls",
			"PlaintextLen": float64(8190), "PlaintextPreview": string(bytes.Repeat([]byte{0x00}, 64)),
		})
		index++
	}

	shown := Trim(flows, 20)
	assert.Len(t, shown, 20)
	for _, flow := range shown {
		assert.GreaterOrEqual(t, int(flow["Time"].(float64)), 160)
	}
}
