package display

import (
	"testing"

	"github.com/rivo/tview"
	"github.com/stretchr/testify/assert"
)

func TestPayloadPanelCreatesPrimitiveForWireAndPlaintext(t *testing.T) {
	assert.IsType(t, &tview.TextView{}, PayloadPanel(PayloadOptions{
		Data: []byte("GET / HTTP/1.1\r\n\r\n"), Kind: PayloadPlaintext, Title: "payload",
	}))
	assert.NotNil(t, PayloadPanel(PayloadOptions{Data: []byte{0x01, 0x02}, Kind: PayloadWire, Title: "wire"}))
}
