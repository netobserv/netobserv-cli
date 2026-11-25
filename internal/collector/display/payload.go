package display

import (
	"fmt"

	hexview "github.com/jmhobbs/tview-hexview"
	collectorformat "github.com/netobserv/network-observability-cli/internal/collector/format"
	"github.com/rivo/tview"
)

const (
	PayloadPlaintext = "plaintext"
	PayloadWire      = "wire"
)

type PayloadOptions struct {
	Data  []byte
	Kind  string
	Title string
}

func PayloadPanel(options PayloadOptions) tview.Primitive {
	if options.Kind == PayloadPlaintext {
		if formatted := collectorformat.PlaintextPayload(options.Data); formatted != "" {
			return textPanel(formatted, options.Title, true)
		}
		if len(options.Data) > 0 {
			hex := hexview.NewHexView(options.Data)
			hex.SetBorder(true).SetTitle(fmt.Sprintf("TLS Plaintext (binary, %dB)", len(options.Data)))
			return hex
		}
		return textPanel("(empty TLS plaintext payload)", options.Title, false)
	}
	if formatted := collectorformat.WireHTTPPayload(options.Data); formatted != "" {
		return textPanel(formatted, options.Title, true)
	}
	hex := hexview.NewHexView(options.Data)
	hex.SetBorder(true).SetTitle(options.Title)
	return hex
}

func textPanel(text, title string, scrollable bool) *tview.TextView {
	view := tview.NewTextView().SetDynamicColors(false).SetWrap(true).SetText(text)
	if scrollable {
		view.SetScrollable(true)
	}
	view.SetBorder(true).SetTitle(title)
	return view
}
