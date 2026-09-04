package cmd

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/netobserv/flowlogs-pipeline/pkg/config"
)

const maxPlaintextUnwrapDepth = 6

var plaintextJSONSkipKeys = map[string]struct{}{
	"RecordType": {}, "Direction": {}, "TLSSource": {}, "Protocol": {},
	"SSLType": {}, "Time": {}, "TimeFlowStartMs": {}, "Pid": {}, "Tgid": {},
	"SrcAddr": {}, "DstAddr": {}, "SrcPort": {}, "DstPort": {}, "PlaintextLen": {},
	"PacketID": {}, "PcapAnnotated": {},
}

var plaintextJSONTextKeys = []string{
	"text", "message", "msg", "body", "content", "data", "response", "result", "error",
}

var embeddedExportJSONMarkers = [][]byte{
	[]byte(`{"RecordType":"plaintext"`),
	[]byte(`{"Direction":"write"`),
	[]byte(`{"Direction":"read"`),
}

// plaintextPayloadBytes returns TLS application bytes after peeling wrappers.
func plaintextPayloadBytes(m config.GenericMap) []byte {
	var raw []byte
	if pt := plaintextFieldString(m); pt != "" {
		if decoded, err := base64.StdEncoding.DecodeString(pt); err == nil && len(decoded) > 0 {
			raw = decoded
		}
	}
	if len(raw) == 0 {
		if preview, ok := m["PlaintextPreview"].(string); ok && preview != "" {
			raw = []byte(preview)
		}
	}
	if len(raw) == 0 {
		return nil
	}
	return unwrapPayloadLayers(raw)
}

func plaintextFieldString(m config.GenericMap) string {
	v, ok := m["Plaintext"]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func unwrapPayloadLayers(data []byte) []byte {
	for i := 0; i < maxPlaintextUnwrapDepth; i++ {
		next := unwrapPayloadLayer(data)
		if bytes.Equal(next, data) {
			break
		}
		data = next
	}
	return data
}

func unwrapPayloadLayer(data []byte) []byte {
	if u := unwrapEmbeddedExportRecord(data); !bytes.Equal(u, data) {
		return u
	}
	if embedded := extractEmbeddedExportJSON(data); len(embedded) > 0 {
		return embedded
	}
	if stripped := stripLeadingApplicationPrefix(data); !bytes.Equal(stripped, data) {
		return stripped
	}
	return data
}

// stripLeadingApplicationPrefix removes a short binary prefix (e.g. OpenSSL ssl_type
// metadata leaked before the userspace buffer) when it is followed by HTTP or JSON text.
func stripLeadingApplicationPrefix(data []byte) []byte {
	if len(data) == 0 || isApplicationTextStart(data) {
		return data
	}
	const maxSkip = 8
	limit := maxSkip
	if len(data)-1 < limit {
		limit = len(data) - 1
	}
	for skip := 1; skip <= limit; skip++ {
		rest := data[skip:]
		if isApplicationTextStart(rest) {
			return rest
		}
	}
	return data
}

func isApplicationTextStart(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	for _, marker := range applicationTextMarkers {
		if bytes.HasPrefix(data, marker) {
			return true
		}
	}
	return false
}

var applicationTextMarkers = [][]byte{
	[]byte("GET "),
	[]byte("POST "),
	[]byte("PUT "),
	[]byte("HEAD "),
	[]byte("HTTP/"),
	[]byte("DELETE "),
	[]byte("OPTIONS "),
	[]byte("PATCH "),
	[]byte("CONNECT "),
	[]byte("TRACE "),
	[]byte("NETOBSERV-"),
	[]byte("{"),
}

// extractEmbeddedExportJSON finds agent export/jsonl records inside gRPC or HTTP/2 frames.
func extractEmbeddedExportJSON(data []byte) []byte {
	for _, marker := range embeddedExportJSONMarkers {
		searchFrom := 0
		for {
			idx := bytes.Index(data[searchFrom:], marker)
			if idx < 0 {
				break
			}
			idx += searchFrom
			objBytes, ok := sliceJSONObject(data, idx)
			if !ok {
				searchFrom = idx + 1
				continue
			}
			if inner := unwrapExportJSONObject(objBytes); len(inner) > 0 && !bytes.Equal(inner, objBytes) {
				return inner
			}
			searchFrom = idx + 1
		}
	}
	return nil
}

func sliceJSONObject(data []byte, start int) ([]byte, bool) {
	if start < 0 || start >= len(data) || data[start] != '{' {
		return nil, false
	}
	depth := 0
	inString := false
	escape := false
	for i := start; i < len(data); i++ {
		c := data[i]
		if inString {
			if escape {
				escape = false
				continue
			}
			switch c {
			case '\\':
				escape = true
			case '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return data[start : i+1], true
			}
		}
	}
	return nil, false
}

func unwrapExportJSONObject(objBytes []byte) []byte {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(objBytes, &obj); err != nil {
		return nil
	}
	if !isExportRecordObject(obj) {
		return nil
	}
	if pt := jsonRawString(obj["Plaintext"]); pt != "" {
		if decoded, err := base64.StdEncoding.DecodeString(pt); err == nil && len(decoded) > 0 {
			return decoded
		}
	}
	if preview := jsonRawString(obj["PlaintextPreview"]); preview != "" {
		return []byte(preview)
	}
	return nil
}

func isExportRecordObject(obj map[string]json.RawMessage) bool {
	if jsonRawString(obj["RecordType"]) == "plaintext" {
		return true
	}
	if _, hasPT := obj["Plaintext"]; hasPT {
		if _, hasDir := obj["Direction"]; hasDir {
			return true
		}
	}
	return false
}

// unwrapEmbeddedExportRecord handles a top-level jsonl/export record payload.
func unwrapEmbeddedExportRecord(data []byte) []byte {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return data
	}
	if inner := unwrapExportJSONObject(trimmed); len(inner) > 0 {
		return inner
	}
	return data
}

// formatPlaintextPayload turns captured TLS bytes into human-readable text for the TUI.
func formatPlaintextPayload(data []byte) string {
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
	display := plaintextDisplayString(data)
	if isGarbageDisplay(display) {
		return ""
	}
	return display
}

// isMeaningfulPlaintextRecord reports whether a plaintext row is worth showing in the TUI.
func isMeaningfulPlaintextRecord(m config.GenericMap) bool {
	return plaintextPreviewForDisplay(m) != ""
}

// enrichPlaintextForExport adds a human-readable PlaintextDisplay field for jsonl consumers.
func enrichPlaintextForExport(m *config.GenericMap) {
	if m == nil {
		return
	}
	display := plaintextPreviewForDisplay(*m)
	if display != "" {
		(*m)["PlaintextDisplay"] = display
	}
}

func extractHTTPBody(data []byte) []byte {
	if len(data) == 0 {
		return data
	}
	if bytes.HasPrefix(data, []byte("HTTP/")) {
		if idx := bytes.Index(data, []byte("\r\n\r\n")); idx >= 0 {
			return data[idx+4:]
		}
		if idx := bytes.Index(data, []byte("\n\n")); idx >= 0 {
			return data[idx+2:]
		}
	}
	return data
}

func extractHTTPRequestLine(data []byte) []byte {
	if len(data) == 0 {
		return data
	}
	switch data[0] {
	case 'G', 'P', 'H', 'D', 'C', 'O', 'T':
	default:
		return data
	}
	if idx := bytes.IndexByte(data, '\n'); idx > 0 {
		line := bytes.TrimSpace(data[:idx])
		if len(line) > 0 && bytes.IndexByte(line, ' ') > 0 {
			return line
		}
	}
	if idx := bytes.Index(data, []byte("\r\n")); idx > 0 {
		return bytes.TrimSpace(data[:idx])
	}
	return data
}

func extractJSONTextBody(data []byte) []byte {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return data
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err == nil {
			return []byte(s)
		}
		return data
	}
	if trimmed[0] != '{' {
		return data
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &obj); err != nil {
		return data
	}
	if isExportRecordObject(obj) {
		return data
	}
	for _, key := range plaintextJSONTextKeys {
		if s := jsonRawString(obj[key]); s != "" && !looksLikeStructuredJSON(s) {
			return []byte(s)
		}
	}
	best := ""
	bestScore := -1
	for key, raw := range obj {
		if _, skip := plaintextJSONSkipKeys[key]; skip {
			continue
		}
		s := jsonRawString(raw)
		if s == "" || looksLikeStructuredJSON(s) || looksLikeBase64Blob(s) {
			continue
		}
		score := printableTextScore(s)
		if score > bestScore {
			bestScore = score
			best = s
		}
	}
	if best != "" {
		return []byte(best)
	}
	return data
}

func isNoisePayload(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	if isTLSRecord(data) {
		return true
	}
	if bytes.HasPrefix(data, []byte("PRI * HTTP/2")) {
		return true
	}
	if bytes.Contains(data, []byte("\x1b[")) {
		return true
	}
	if containsAgentExportLeak(data) {
		return true
	}
	if len(data) <= 8 && printableRatio(data) < 0.5 {
		return true
	}
	if bytes.HasPrefix(data, []byte{0x00, 0x00}) && printableRatio(data) < 0.72 {
		return true
	}
	if printableRatio(data) < 0.35 && len(data) > 16 {
		return true
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &obj); err == nil {
			if isExportRecordObject(obj) || isPacketExportObject(obj) {
				return true
			}
		}
	}
	return false
}

func isTLSRecord(data []byte) bool {
	if len(data) < 3 || data[1] != 0x03 {
		return false
	}
	switch data[0] {
	case 0x14, 0x15, 0x16, 0x17:
		return data[2] == 0x01 || data[2] == 0x03 || data[2] == 0x04
	default:
		return false
	}
}

func containsAgentExportLeak(data []byte) bool {
	if bytes.Contains(data, []byte(`"RecordType":"plaintext"`)) {
		return true
	}
	if bytes.Contains(data, []byte(`"Plaintext"`)) && bytes.Contains(data, []byte(`"Direction"`)) {
		return true
	}
	if bytes.Contains(data, []byte(`"Bytes"`)) && bytes.Contains(data, []byte(`"Data"`)) {
		return true
	}
	return false
}

func isPacketExportObject(obj map[string]json.RawMessage) bool {
	_, hasBytes := obj["Bytes"]
	_, hasData := obj["Data"]
	return hasBytes && hasData
}

func plaintextDisplayString(data []byte) string {
	s := strings.ToValidUTF8(string(data), "\uFFFD")
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

func isGarbageDisplay(s string) bool {
	if s == "" {
		return true
	}
	if strings.Contains(s, `\x`) {
		return true
	}
	if strings.Contains(s, `{"Bytes":`) || strings.Contains(s, `{"Direction":`) {
		return true
	}
	if strings.Contains(s, "\x1b[") {
		return true
	}
	if printableRatio([]byte(s)) < 0.72 && len(s) > 24 {
		return true
	}
	if len(strings.TrimSpace(s)) < 6 && !looksLikeShortPlaintext(s) {
		return true
	}
	return false
}

var shortPlaintextHTTPPrefixes = []string{
	"HTTP/", "GET ", "POST ", "PUT ", "DELETE ", "HEAD ", "OPTIONS ", "PATCH ",
}

func looksLikeShortPlaintext(s string) bool {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return false
	}
	for _, prefix := range shortPlaintextHTTPPrefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	if trimmed[0] == '{' || trimmed[0] == '[' {
		return true
	}
	return isPlainIdentifierText(trimmed) && len(trimmed) >= 2
}

func isPlainIdentifierText(s string) bool {
	for _, r := range s {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '/' || r == '.' || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func printableRatio(data []byte) float64 {
	if len(data) == 0 {
		return 0
	}
	printable := 0
	for _, b := range data {
		if b == '\t' || b == '\n' || b == '\r' || (b >= 32 && b < 127) {
			printable++
		}
	}
	return float64(printable) / float64(len(data))
}

func jsonRawString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return ""
}

func looksLikeStructuredJSON(s string) bool {
	t := strings.TrimSpace(s)
	return strings.HasPrefix(t, "{") || strings.HasPrefix(t, "[")
}

func looksLikeBase64Blob(s string) bool {
	if len(s) < 32 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '+', r == '/', r == '=':
		default:
			return false
		}
	}
	_, err := base64.StdEncoding.DecodeString(s)
	return err == nil
}

func printableTextScore(s string) int {
	if s == "" {
		return -1
	}
	score := len(s)
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' {
			continue
		}
		if r < 32 || !unicode.IsPrint(r) {
			score -= 4
		}
	}
	return score
}

func plaintextPreviewForDisplay(m config.GenericMap, maxLen ...int) string {
	payload := plaintextPayloadBytes(m)
	if len(payload) == 0 {
		return ""
	}
	display := formatPlaintextPayload(payload)
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
	return fmt.Sprintf("%s...", s[:maxLen])
}
