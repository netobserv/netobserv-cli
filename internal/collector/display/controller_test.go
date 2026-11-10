package display

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/stretchr/testify/assert"
)

func TestControllerBuildsRootAndRoutesInput(t *testing.T) {
	screen := tcell.NewSimulationScreen("UTF-8")
	screen.SetSize(80, 20)
	input := 0
	root := tview.NewTextView().SetText("collector")
	controller := NewController(
		func() tview.Primitive { return root },
		func(event *tcell.EventKey) *tcell.EventKey {
			input++
			return event
		},
	)
	controller.Application.SetScreen(screen)

	// The application loop is intentionally exercised instead of calling the
	// capture callback directly: this verifies the same setup used by Run.
	done := make(chan error, 1)
	go func() { done <- controller.Run() }()

	controller.Application.QueueEvent(tcell.NewEventKey(tcell.KeyRune, 'x', tcell.ModNone))
	controller.Application.QueueEvent(tcell.NewEventKey(tcell.KeyCtrlC, 0, tcell.ModNone))

	assert.NoError(t, <-done)
	assert.Equal(t, 2, input)
}
