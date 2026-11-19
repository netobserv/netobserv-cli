package collector

import (
	"testing"

	"github.com/netobserv/flowlogs-pipeline/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestAppendFlowSkipsAllFlowsWhilePaused(t *testing.T) {
	setup(t)
	capture = Packet
	options = "enable_openssl=true"
	ui.paused = true

	appendFlow(config.GenericMap{"Time": float64(1), "Data": "d2lyZQ=="})
	appendFlow(config.GenericMap{
		"Time": float64(2), "RecordType": "plaintext", "PlaintextPreview": "hello", "PlaintextLen": float64(5),
	})
	assert.Empty(t, ui.FlowBuffer.Snapshot(string(capture), plaintextCaptureEnabled()))
	assert.Empty(t, ui.selectedData)
}
