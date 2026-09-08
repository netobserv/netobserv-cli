package cmd

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/netobserv/flowlogs-pipeline/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestPlaintextPayloadBytesPrefersFullPlaintext(t *testing.T) {
	full := []byte("HTTP/1.1 200 OK\r\n\r\nktls-test-pod response body")
	m := config.GenericMap{
		"Plaintext":        base64.StdEncoding.EncodeToString(full),
		"PlaintextPreview": "HTTP/1.1 200 OK",
		"PlaintextLen":     float64(len(full)),
	}
	assert.Equal(t, full, plaintextPayloadBytes(m))
}

func TestFormatPlaintextPayloadHTTPResponse(t *testing.T) {
	raw := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\n\r\nktls-test-pod path=/api/items")
	assert.Equal(t, "ktls-test-pod path=/api/items", formatPlaintextPayload(raw))
}

func TestFormatPlaintextPayloadHealthzResponse(t *testing.T) {
	raw := []byte("HTTP/1.1 200 OK\r\nDate: Thu, 25 Jun 2026 16:27:10 GMT\r\nContent-Length: 2\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nOK")
	assert.Equal(t, "OK", formatPlaintextPayload(raw))
}

func TestFormatPlaintextPayloadJSONMessageField(t *testing.T) {
	raw := []byte(`{"message":"hello from api","status":"ok"}`)
	assert.Equal(t, "hello from api", formatPlaintextPayload(raw))
}

func TestFormatPlaintextPayloadHTTPRequestLine(t *testing.T) {
	raw := []byte("GET /health HTTP/1.1\r\nHost: :8080\r\nUser-Agent: Go-http-client/1.1\r\n\r\n")
	assert.Equal(t, "GET /health HTTP/1.1", formatPlaintextPayload(raw))
}

func TestFormatPlaintextPayloadStripsOpenSSLMetadataPrefix(t *testing.T) {
	// Observed on openssl captures: ssl_type/metadata before decrypted HTTP (2026-06-26T132343Z.jsonl).
	request := append([]byte{0x05, 0x00, 0x00, 0x00}, []byte("GET /healthz HTTP/1.1\r\nHost: 10.244.2.24:8443\r\n\r\n")...)
	assert.Equal(t, "GET /healthz HTTP/1.1", formatPlaintextPayload(request))

	response := append([]byte{0x05, 0x00, 0x00, 0x00}, []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\nContent-Type: text/plain\r\n\r\nOK")...)
	assert.Equal(t, "OK", formatPlaintextPayload(response))
}

func TestPlaintextPayloadBytesStripsOpenSSLMetadataPrefix(t *testing.T) {
	raw := append([]byte{0x05, 0x00, 0x00, 0x00}, []byte("NETOBSERV-OPENSSL probe")...)
	m := config.GenericMap{
		"Plaintext": base64.StdEncoding.EncodeToString(raw),
	}
	assert.Equal(t, []byte("NETOBSERV-OPENSSL probe"), plaintextPayloadBytes(m))
}

func TestFormatPlaintextPayloadFromCaptureJSONL132343Z(t *testing.T) {
	jsonl := filepath.Join("..", "output", "plaintext", "2026-06-26T132343Z.jsonl")
	if _, err := os.Stat(jsonl); err != nil {
		t.Skip("capture jsonl not present:", err)
	}
	data, err := os.ReadFile(jsonl)
	if err != nil {
		t.Fatal(err)
	}
	var readLine, writeLine string
	for _, line := range splitJSONLLines(data) {
		if line == "" {
			continue
		}
		var m config.GenericMap
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatal(err)
		}
		switch m["Direction"] {
		case "read":
			if readLine == "" {
				readLine = line
			}
		case "write":
			if writeLine == "" {
				writeLine = line
			}
		}
	}
	var readMap, writeMap config.GenericMap
	if err := json.Unmarshal([]byte(readLine), &readMap); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(writeLine), &writeMap); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "GET /healthz HTTP/1.1", plaintextPreviewForDisplay(readMap))
	// Full write capture ends mid-headers (no body after \r\n\r\n); status line is still readable.
	assert.Equal(t, "HTTP/1.1 200 OK", plaintextPreviewForDisplay(writeMap))
}

func TestFormatPlaintextPayloadUnwrapsExportRecordJSON(t *testing.T) {
	body := []byte("ktls-test-pod path=/api/items method=GET")
	export := config.GenericMap{
		"RecordType":       "plaintext",
		"Plaintext":        base64.StdEncoding.EncodeToString(body),
		"PlaintextPreview": string(body),
		"Direction":        "write",
		"TLSSource":        "ktls",
	}
	line, err := json.Marshal(export)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, string(body), formatPlaintextPayload(line))
}

func TestFormatPlaintextPayloadUnwrapsGRPCEmbeddedExportJSON(t *testing.T) {
	inner := []byte("GET /readyz HTTP/1.1\r\nHost: 127.0.0.1:2381\r\n\r\n")
	export := config.GenericMap{
		"Direction":        "write",
		"Plaintext":        base64.StdEncoding.EncodeToString(inner),
		"PlaintextPreview": string(inner),
		"RecordType":       "plaintext",
	}
	exportJSON, err := json.Marshal(export)
	if err != nil {
		t.Fatal(err)
	}
	// gRPC DATA-like prefix observed in 2026-06-25T162701Z.jsonl
	wrapped := append([]byte("\x00\x00\x08\x01\x04\x00\x00\x00\x03\xff\xff\xff\xff\xff\xff\xff\x00\x00\x80\x00\x01\x00\x00\x00\x03\x00\x00\x00\x00\x80\n\x99\x01\x12\x99\x01"), exportJSON...)
	assert.Equal(t, "GET /readyz HTTP/1.1", formatPlaintextPayload(wrapped))
}

func TestFormatPlaintextPayloadNoiseTLSRecord(t *testing.T) {
	raw := []byte{0x17, 0x03, 0x03, 0x00, 0x19, 0x4b, 0xfb, 0x72}
	assert.Equal(t, "", formatPlaintextPayload(raw))
}

func TestFormatPlaintextPayloadNoiseTinyBinary(t *testing.T) {
	raw := []byte{0x00, 0x00, 0x00, 0x03}
	assert.Equal(t, "", formatPlaintextPayload(raw))
}

func TestIsMeaningfulPlaintextRecordFromCaptureJSONL(t *testing.T) {
	jsonl := filepath.Join("..", "output", "plaintext", "2026-06-25T162701Z.jsonl")
	if _, err := os.Stat(jsonl); err != nil {
		t.Skip("capture jsonl not present:", err)
	}
	data, err := os.ReadFile(jsonl)
	if err != nil {
		t.Fatal(err)
	}

	meaningful := 0
	noise := 0
	for _, line := range splitJSONLLines(data) {
		if line == "" {
			continue
		}
		var m config.GenericMap
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatal(err)
		}
		if isMeaningfulPlaintextRecord(m) {
			meaningful++
		} else {
			noise++
		}
	}
	assert.Equal(t, 2479, meaningful+noise)
	// 2026-06-25T162701Z.jsonl: kube/health probe HTTP lines; rest is kTLS noise
	// (gRPC export feedback, TLS records, binary framing). Prefix stripping surfaces
	// a few more openssl healthz rows that previously looked binary.
	assert.Equal(t, 90, meaningful)
	assert.Equal(t, 2389, noise)
}

func splitJSONLLines(data []byte) []string {
	var lines []string
	start := 0
	for i := 0; i < len(data); i++ {
		if data[i] != '\n' {
			continue
		}
		lines = append(lines, string(data[start:i]))
		start = i + 1
	}
	if start < len(data) {
		lines = append(lines, string(data[start:]))
	}
	return lines
}
