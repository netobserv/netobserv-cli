package display

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func renderPrimitive(t *testing.T, primitive tview.Primitive) string {
	t.Helper()

	screen := tcell.NewSimulationScreen("UTF-8")
	screen.SetSize(120, 30)
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

func press(t *testing.T, primitive tview.Primitive, key tcell.Key) {
	t.Helper()
	input := primitive.InputHandler()
	if input == nil {
		t.Fatalf("primitive %T has no input handler", primitive)
	}
	input(tcell.NewEventKey(key, 0, tcell.ModNone), func(tview.Primitive) {})
}
