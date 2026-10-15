package display

import (
	"time"
)

// Runtime coordinates display refreshes without depending on collector state.
// The collector supplies the small callbacks needed for capture-specific work.
type Runtime struct {
	Ended       func() bool
	IsMetrics   func() bool
	FPS         func() int
	Sleep       func(time.Duration)
	UpdateState func()
	UpdateData  func()
	Draw        func()
	PrintRows   func()
}

func (r Runtime) Heartbeat() {
	sleep := r.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	for {
		if r.Ended() {
			return
		}
		r.UpdateState()
		r.UpdateData()
		r.Draw()
		sleep(time.Second / time.Duration(max(1, r.FPS())))
	}
}

func (r Runtime) BackgroundHeartbeat() {
	sleep := r.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	for {
		if r.Ended() {
			return
		}
		if !r.IsMetrics() {
			r.UpdateData()
			r.PrintRows()
		}
		sleep(5 * time.Second)
	}
}
