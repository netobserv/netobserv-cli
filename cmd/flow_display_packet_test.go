package cmd

import (
	"bytes"
	"encoding/base64"
	"testing"

	"github.com/netobserv/flowlogs-pipeline/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestAppendPlaintextColumns(t *testing.T) {
	setup(t)
	capture = Packet
	options = "enable_openssl=true"

	cols := appendPlaintextColumns([]string{"EndTime", "SrcAddr"})
	assert.Equal(t, []string{"EndTime", "SrcAddr", "RecordType", "Direction", "PlaintextPreview"}, cols)

	selectedColumns = []string{"EndTime", "SrcAddr"}
	cols = appendPlaintextColumns([]string{"EndTime", "SrcAddr"})
	assert.Equal(t, []string{"EndTime", "SrcAddr"}, cols)
	selectedColumns = nil

	capture = Flow
	cols = appendPlaintextColumns([]string{"EndTime"})
	assert.Equal(t, []string{"EndTime"}, cols)
}

func TestPlaintextPreviewColumnWidth(t *testing.T) {
	setup(t)
	assert.GreaterOrEqual(t, toColWidth("PlaintextPreview"), 120)
}

func TestTrimFlowsToShowCountUnified(t *testing.T) {
	setup(t)
	capture = Packet

	var flows []config.GenericMap
	for i := range 50 {
		m := config.GenericMap{
			"Index": i,
			"Time":  float64(i),
		}
		if i%5 == 0 {
			m["RecordType"] = "plaintext"
			m["PlaintextPreview"] = "GET /"
		} else {
			m["Data"] = "d2lyZQ=="
		}
		flows = append(flows, m)
	}

	shown := trimFlowsToShowCount(flows, 10)
	assert.Len(t, shown, 10)
	for _, f := range shown {
		ts := int(f["Time"].(float64))
		assert.GreaterOrEqual(t, ts, 40)
	}
	// Last 10 times 40-49: two plaintext (40, 45) and eight wire rows.
	plaintextCount := 0
	for _, f := range shown {
		if isPlaintextRecord(f) {
			plaintextCount++
		}
	}
	assert.Equal(t, 2, plaintextCount)
}

func TestTrimPacketCaptureFlowsNewestByTime(t *testing.T) {
	setup(t)
	capture = Packet
	options = "enable_openssl=true,enable_gotls=true,enable_ktls=true"

	var flows []config.GenericMap
	idx := 0
	add := func(m config.GenericMap) {
		m["Index"] = idx
		idx++
		flows = append(flows, m)
	}
	for i := range 40 {
		add(config.GenericMap{
			"Time": float64(i),
			"Data": "d2lyZQ==",
		})
	}
	add(config.GenericMap{
		"Time":             float64(10),
		"RecordType":       "plaintext",
		"TLSSource":        "gotls",
		"PlaintextPreview": "GET /health HTTP/1.1\r\n\r\n",
	})

	shown := trimFlowsToShowCount(flows, 5)
	assert.Len(t, shown, 5)
	for _, f := range shown {
		ts := int(f["Time"].(float64))
		assert.GreaterOrEqual(t, ts, 35, "display should keep newest rows by time only")
	}
}

func TestTrimPacketCaptureFlowsPlaintextOnly(t *testing.T) {
	setup(t)
	capture = Packet
	options = "enable_openssl=true"

	var flows []config.GenericMap
	for i := range 30 {
		flows = append(flows, config.GenericMap{
			"Index":            i,
			"Time":             float64(i),
			"RecordType":       "plaintext",
			"TLSSource":        "openssl",
			"PlaintextPreview": "GET /health HTTP/1.1\r\n\r\n",
		})
	}

	shown := trimFlowsToShowCount(flows, 10)
	assert.Len(t, shown, 10)
	for _, f := range shown {
		assert.True(t, isPlaintextRecord(f))
		ts := int(f["Time"].(float64))
		assert.GreaterOrEqual(t, ts, 20)
	}
}

func TestTrimPacketCaptureFlowsBinaryPlaintext(t *testing.T) {
	setup(t)
	capture = Packet
	options = "enable_ktls=true"

	var flows []config.GenericMap
	for i := range 50 {
		flows = append(flows, config.GenericMap{
			"Index":            i,
			"Time":             float64(i),
			"RecordType":       "plaintext",
			"TLSSource":        "ktls",
			"Direction":        "write",
			"PlaintextLen":     float64(8190),
			"PlaintextPreview": string(bytes.Repeat([]byte{0x00, 0x01}, 128)),
		})
	}

	shown := trimFlowsToShowCount(flows, 10)
	assert.Len(t, shown, 10)
	for _, f := range shown {
		assert.True(t, isPlaintextRecord(f))
	}
}

func TestPlaintextTablePreviewBinarySummary(t *testing.T) {
	preview := plaintextTablePreview(config.GenericMap{
		"TLSSource":        "ktls",
		"Direction":        "write",
		"PlaintextLen":     float64(8190),
		"PlaintextPreview": "\x00\x01",
	}, 80)
	assert.Equal(t, "<ktls write 8190B binary>", preview)
}

func TestPlaintextRawBytesPrefersFullPayload(t *testing.T) {
	full := bytes.Repeat([]byte{0x17, 0x03, 0x03}, 26)
	m := config.GenericMap{
		"Plaintext":        base64.StdEncoding.EncodeToString(full),
		"PlaintextPreview": "short",
		"PlaintextLen":     float64(len(full)),
	}
	assert.Equal(t, full, plaintextRawBytes(m))
}

func TestFormatPlaintextPayloadBinaryIsEmpty(t *testing.T) {
	data := bytes.Repeat([]byte{0x17, 0x03, 0x03}, 10)
	assert.Empty(t, formatPlaintextPayload(data))
}

func TestTrimPacketCaptureFlowsWireAndPlaintextMix(t *testing.T) {
	setup(t)
	capture = Packet
	options = "enable_openssl=true,enable_ktls=true"

	var flows []config.GenericMap
	idx := 0
	for i := range 80 {
		flows = append(flows, config.GenericMap{
			"Index": idx,
			"Time":  float64(i),
			"Data":  "d2lyZQ==",
		})
		idx++
	}
	for i := range 80 {
		flows = append(flows, config.GenericMap{
			"Index":            idx,
			"Time":             float64(100 + i),
			"RecordType":       "plaintext",
			"TLSSource":        "ktls",
			"PlaintextLen":     float64(8190),
			"PlaintextPreview": string(bytes.Repeat([]byte{0x00}, 64)),
		})
		idx++
	}

	shown := trimFlowsToShowCount(flows, 20)
	assert.Len(t, shown, 20)
	for _, f := range shown {
		ts := int(f["Time"].(float64))
		assert.GreaterOrEqual(t, ts, 160)
	}
}

func TestAppendFlowSkipsAllFlowsWhilePaused(t *testing.T) {
	setup(t)
	capture = Packet
	options = "enable_openssl=true"
	paused = true

	AppendFlow(config.GenericMap{"Time": float64(1), "Data": "d2lyZQ=="})
	AppendFlow(config.GenericMap{
		"Time":             float64(2),
		"RecordType":       "plaintext",
		"PlaintextPreview": "hello",
		"PlaintextLen":     float64(5),
	})
	assert.Empty(t, lastFlows)
	assert.Empty(t, lastWirePackets)
	assert.Empty(t, lastPlaintextFlows)
	assert.Empty(t, selectedData)
}

func TestHexPanelTitle(t *testing.T) {
	capture = Packet
	selectedPayloadKind = payloadKindPlaintext
	assert.Equal(t, "TLS Plaintext", hexPanelTitle())
	selectedPayloadKind = payloadKindWire
	assert.Equal(t, "Wire payload", hexPanelTitle())
}

func TestPlaintextDisplayString(t *testing.T) {
	assert.Equal(t, "GET / HTTP/1.1\nHost: example.com", plaintextDisplayString([]byte("GET / HTTP/1.1\nHost: example.com")))
	assert.Equal(t, `{"msg":"hi"}`, plaintextDisplayString([]byte(`{"msg":"hi"}`)))
	assert.Equal(t, "hello\\x00world", plaintextDisplayString([]byte("hello\x00world")))
}
