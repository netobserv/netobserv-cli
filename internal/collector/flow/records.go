package flow

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"

	"github.com/netobserv/flowlogs-pipeline/pkg/config"
	collectorpacket "github.com/netobserv/network-observability-cli/internal/collector/packet"
)

// ParseRecord decodes one flow pipeline record. Keeping record decoding with
// flow storage/capture code avoids making the collector command own format
// details.
func ParseRecord(data []byte) (config.GenericMap, error) {
	record := config.GenericMap{}
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, err
	}
	return record, nil
}

// Visible returns the records that should be rendered in the live table.
// Existing records are retained for flow captures so a selected row does not
// disappear immediately when it leaves the capture buffer.
func Visible(records, existing []config.GenericMap, filters []string, limit int, packetCapture bool) []config.GenericMap {
	if !packetCapture {
		indexes := make(map[int]struct{}, len(records))
		for _, record := range records {
			if index, ok := record["Index"].(int); ok {
				indexes[index] = struct{}{}
			}
		}
		missing := make([]config.GenericMap, 0, len(existing))
		for _, record := range existing {
			if index, ok := record["Index"].(int); ok {
				if _, found := indexes[index]; !found {
					missing = append(missing, record)
				}
			}
		}
		records = append(missing, records...)
	}

	if len(filters) > 0 {
		filtered := make([]config.GenericMap, 0, len(records))
		for _, record := range records {
			match := true
			for _, expression := range slices.Clone(filters) {
				matched, _ := regexp.MatchString(expression, fmt.Sprintf("%v", record))
				if !matched {
					match = false
					break
				}
			}
			if match {
				filtered = append(filtered, record)
			}
		}
		records = filtered
	}

	if packetCapture {
		return collectorpacket.Trim(records, limit)
	}
	if len(records) > limit {
		return records[len(records)-limit:]
	}
	return records
}
