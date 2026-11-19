package display

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRuntimeHeartbeatUpdatesUntilCaptureEnds(t *testing.T) {
	ended := false
	updates := 0
	draws := 0
	sleeps := 0

	Runtime{
		Ended: func() bool { return ended },
		FPS:   func() int { return 0 },
		UpdateState: func() {
			updates++
			ended = true
		},
		UpdateData: func() { draws++ },
		Draw:       func() { draws++ },
		Sleep:      func(time.Duration) { sleeps++ },
	}.Heartbeat()

	assert.Equal(t, 1, updates)
	assert.Equal(t, 2, draws)
	assert.Equal(t, 1, sleeps)
}

func TestRuntimeBackgroundHeartbeatSkipsFlowUpdatesForMetrics(t *testing.T) {
	ended := false
	updates := 0
	rows := 0

	Runtime{
		Ended:     func() bool { return ended },
		IsMetrics: func() bool { return true },
		UpdateData: func() {
			updates++
			ended = true
		},
		PrintRows: func() { rows++ },
		Sleep:     func(time.Duration) { ended = true },
	}.BackgroundHeartbeat()

	assert.Zero(t, updates)
	assert.Zero(t, rows)

	ended = false
	flowUpdates := 0
	flowRows := 0
	Runtime{
		Ended:     func() bool { return ended },
		IsMetrics: func() bool { return false },
		UpdateData: func() {
			flowUpdates++
			ended = true
		},
		PrintRows: func() { flowRows++ },
		Sleep:     func(time.Duration) {},
	}.BackgroundHeartbeat()

	assert.Equal(t, 1, flowUpdates)
	assert.Equal(t, 1, flowRows)
}
