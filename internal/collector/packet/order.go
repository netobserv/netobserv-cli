package packet

import (
	"slices"

	"github.com/netobserv/flowlogs-pipeline/pkg/config"
	collectorformat "github.com/netobserv/network-observability-cli/internal/collector/format"
)

func TimeMs(record config.GenericMap) float64 {
	if value := collectorformat.ToFloat64(record, "TimeFlowStartMs"); value > 0 {
		return value
	}
	return collectorformat.ToFloat64(record, "Time") * 1000
}

func Index(record config.GenericMap) int {
	index, _ := record["Index"].(int)
	return index
}

func Less(a, b config.GenericMap) bool {
	if left, right := TimeMs(a), TimeMs(b); left != right {
		return left < right
	}
	return Index(a) < Index(b)
}

func Sort(records []config.GenericMap) {
	slices.SortStableFunc(records, func(a, b config.GenericMap) int {
		if Less(a, b) {
			return -1
		}
		if Less(b, a) {
			return 1
		}
		return 0
	})
}

func IsWireRecord(record config.GenericMap) bool {
	_, ok := record["Data"]
	return ok
}

func Trim(records []config.GenericMap, limit int) []config.GenericMap {
	if limit <= 0 {
		return nil
	}
	if len(records) <= limit {
		return records
	}
	Sort(records)
	return records[len(records)-limit:]
}
