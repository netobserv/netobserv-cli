package format

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/netobserv/flowlogs-pipeline/pkg/config"
)

func PlaintextPayload(data []byte) string {
	data = unwrapPayloadLayers(data)
	for i := 0; i < 3; i++ {
		next := extractHTTPBody(data)
		next = extractHTTPRequestLine(next)
		next = extractJSONTextBody(next)
		if bytes.Equal(next, data) {
			break
		}
		data = next
	}
	if isNoisePayload(data) {
		return ""
	}
	display := PlaintextDisplayString(data)
	if isGarbageDisplay(display) {
		return ""
	}
	return display
}

func PlaintextPreviewForDisplay(m config.GenericMap, maxLen ...int) string {
	payload := PlaintextPayloadBytes(m)
	if len(payload) == 0 {
		return ""
	}
	display := PlaintextPayload(payload)
	if display == "" {
		return ""
	}
	limit := 120
	if len(maxLen) > 0 && maxLen[0] > 0 {
		limit = maxLen[0]
	}
	return ellipsizePlaintextDisplay(display, limit)
}

func ellipsizePlaintextDisplay(s string, maxLen int) string {
	if maxLen <= 0 || len(s) <= maxLen {
		return s
	}
	// Trim to maxLen without splitting a multi-byte UTF-8 rune at the boundary,
	// which would otherwise emit an invalid trailing character.
	cut := maxLen
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return fmt.Sprintf("%s...", s[:cut])
}

// PlaintextDisplayString renders raw payload bytes as a printable string,
// preserving valid UTF-8 and escaping non-printable control bytes.
func PlaintextDisplayString(data []byte) string {
	s := strings.ToValidUTF8(string(data), "�")
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\n', '\r', '\t':
			b.WriteRune(r)
		default:
			if r >= 32 && r < 127 || r > 127 {
				b.WriteRune(r)
			} else {
				fmt.Fprintf(&b, "\\x%02x", r)
			}
		}
	}
	return b.String()
}
