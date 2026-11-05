package collector

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPacketPayloadPanelTitle(t *testing.T) {
	capture = Packet
	ui.selectedPayloadKind = payloadKindPlaintext
	assert.Equal(t, "TLS Plaintext", hexPanelTitle())

	ui.selectedPayloadKind = payloadKindWire
	assert.Equal(t, "Wire payload", hexPanelTitle())
}
