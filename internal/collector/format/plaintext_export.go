package format

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/netobserv/flowlogs-pipeline/pkg/config"
)

func WriteJSONLine(w io.Writer, value config.GenericMap) error {
	line, err := json.Marshal(value)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	_, err = w.Write(line)
	return err
}

func IsMeaningfulPlaintextRecord(m config.GenericMap) bool {
	return PlaintextPreviewForDisplay(m) != ""
}

// EnrichPlaintextForExport adds a human-readable PlaintextDisplay field for jsonl consumers.
func EnrichPlaintextForExport(m *config.GenericMap) {
	if m == nil {
		return
	}
	display := PlaintextPreviewForDisplay(*m)
	if display != "" {
		(*m)["PlaintextDisplay"] = display
	}
}

// PlaintextTablePreview returns a human-readable column value for any plaintext row.
func PlaintextTablePreview(m config.GenericMap, maxLen int) string {
	if p := PlaintextPreviewForDisplay(m, maxLen); p != "" {
		return p
	}
	return PlaintextBinarySummary(m)
}

// PlaintextBinarySummary returns a compact description for binary plaintext data.
func PlaintextBinarySummary(m config.GenericMap) string {
	src, _ := m["TLSSource"].(string)
	if src == "" {
		src = "tls"
	}
	dir, _ := m["Direction"].(string)
	nbytes := plaintextLen(m)
	if dir != "" {
		return fmt.Sprintf("<%s %s %dB binary>", src, dir, nbytes)
	}
	return fmt.Sprintf("<%s %dB binary>", src, nbytes)
}

func plaintextLen(m config.GenericMap) int {
	switch v := m["PlaintextLen"].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	default:
		if b := PlaintextPayloadBytes(m); len(b) > 0 {
			return len(b)
		}
	}
	return 0
}
