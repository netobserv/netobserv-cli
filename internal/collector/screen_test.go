package collector

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	collectordisplay "github.com/netobserv/network-observability-cli/internal/collector/display"
	"github.com/rivo/tview"
	"github.com/stretchr/testify/assert"
)

func renderCollectorScreen(t *testing.T, primitive tview.Primitive) string {
	t.Helper()

	screen := tcell.NewSimulationScreen("UTF-8")
	screen.SetSize(160, 40)
	app := tview.NewApplication().SetScreen(screen).SetRoot(primitive, true)
	app.ForceDraw()

	cells, width, height := screen.GetContents()
	lines := make([]string, height)
	for row := 0; row < height; row++ {
		var line strings.Builder
		for column := 0; column < width; column++ {
			line.WriteString(string(cells[row*width+column].Runes))
		}
		lines[row] = strings.TrimRight(line.String(), " ")
	}
	return strings.Join(lines, "\n")
}

func TestFlowScreenRendersMainComponents(t *testing.T) {
	setup(t)
	ui.App = nil

	output := renderCollectorScreen(t, getPages())

	assert.Contains(t, output, "Flow Capture")
	assert.Contains(t, output, "Flows")
	assert.Contains(t, output, "Live table filters")
	assert.Contains(t, output, "Manage columns")
}

func TestMetricsScreenRendersMainComponents(t *testing.T) {
	setup(t)
	capture = Metric
	ui.App = nil
	ui.Graphs = nil

	output := renderCollectorScreen(t, getPages())

	assert.Contains(t, output, "Metric Capture")
	assert.Contains(t, output, "Time range")
	assert.Contains(t, output, "Manage panels")
	assert.Contains(t, output, "Showing")
}

func TestFlowKeyboardShortcutsUpdateState(t *testing.T) {
	setup(t)
	ui.App = nil
	ui.PlayPauseButton = tview.NewButton(collectordisplay.PlayPauseText(false))
	ui.focus = "inputField"

	assert.Nil(t, handleFlowKey(tcell.NewEventKey(tcell.KeyCtrlSpace, 0, tcell.ModNone)))
	assert.True(t, ui.paused)

	assert.Nil(t, handleFlowKey(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)))
	assert.Equal(t, "table", ui.focus)

	ui.selectedData = []byte("payload")
	assert.Nil(t, handleFlowKey(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)))
	assert.Equal(t, "hex", ui.focus)

	assert.Nil(t, handleFlowKey(tcell.NewEventKey(tcell.KeyESC, 0, tcell.ModNone)))
	assert.Empty(t, ui.selectedData)

	ui.focus = "inputField"
	assert.Nil(t, handleFlowKey(tcell.NewEventKey(tcell.KeyCtrlD, 0, tcell.ModNone)))
	assert.Nil(t, handleFlowKey(tcell.NewEventKey(tcell.KeyCtrlE, 0, tcell.ModNone)))
}

func TestMetricsKeyboardShortcutsPause(t *testing.T) {
	setup(t)
	capture = Metric
	ui.App = nil
	ui.PlayPauseButton = tview.NewButton(collectordisplay.PlayPauseText(false))

	assert.Nil(t, handleMetricKey(tcell.NewEventKey(tcell.KeyCtrlSpace, 0, tcell.ModNone)))
	assert.True(t, ui.paused)
	assert.Equal(t, collectordisplay.PlayPauseText(true), ui.PlayPauseButton.GetLabel())
}
