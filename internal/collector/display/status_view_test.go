package display

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInfoRowReturnsLiveStatusViews(t *testing.T) {
	_, _, duration, size := InfoRow(&InfoRowOptions{
		DurationText: "Duration: 0s",
		CaptureSize:  "Capture size: 0B",
	})

	duration.SetText("Duration: 3s")
	size.SetText("Capture size: 12KB")

	assert.Equal(t, "Duration: 3s", duration.GetText(true))
	assert.Equal(t, "Capture size: 12KB", size.GetText(true))
}
