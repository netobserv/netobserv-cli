package display

import (
	"fmt"
	"slices"
	"time"

	"github.com/jpillora/sizestr"
	"github.com/netobserv/flowlogs-pipeline/pkg/config"
	"github.com/rivo/tview"
)

// SortAndTrim appends a record, keeps the supplied ordering, and retains only
// the newest maxLen records. The display package owns this generic buffer
// operation; callers provide the capture-specific ordering rule.
func SortAndTrim(buffer *[]config.GenericMap, record config.GenericMap, maxLen int, less func(config.GenericMap, config.GenericMap) bool) {
	*buffer = append(*buffer, record)
	slices.SortStableFunc(*buffer, func(a, b config.GenericMap) int {
		if less(a, b) {
			return -1
		}
		if less(b, a) {
			return 1
		}
		return 0
	})
	if len(*buffer) > maxLen {
		*buffer = (*buffer)[len(*buffer)-maxLen:]
	}
}

// AddPlaintextColumns adds the agent's plaintext fields to packet displays.
func AddPlaintextColumns(cols []string, cfg Config, packetCapture, enabled bool, selected []string) []string {
	if !packetCapture || !enabled || len(selected) > 0 {
		return cols
	}
	if !slices.Contains(cols, "RecordType") {
		cols = append(cols, "RecordType")
	}
	for _, column := range cfg.Columns {
		if column.Feature != "plaintextCapture" || column.Field == "" || slices.Contains(cols, column.ID) {
			continue
		}
		cols = append(cols, column.ID)
	}
	return cols
}

// Modal centers a primitive with the requested dimensions.
func Modal(content tview.Primitive, width, height int) tview.Primitive {
	return tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(content, height, 1, true).
			AddItem(nil, 0, 1, false), width, 1, true).
		AddItem(nil, 0, 1, false)
}

func FPSText(fps int) string { return fmt.Sprintf("FPS: %d", fps) }

func PlayPauseText(paused bool) string {
	if paused {
		return "⏸︎"
	}
	return "⏵︎"
}

func DurationText(now, started time.Time) string {
	return fmt.Sprintf("Duration: %s ", now.Sub(started).Round(time.Second))
}

func CaptureSize(capture string, totalBytes int64) string {
	if capture == "Metric" {
		return ""
	}
	return fmt.Sprintf("Capture size: %s", sizestr.ToString(totalBytes))
}
