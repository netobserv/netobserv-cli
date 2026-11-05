package display

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/netobserv/flowlogs-pipeline/pkg/config"
	"github.com/rivo/tview"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInfoRowRendersStatusAndHandlesPause(t *testing.T) {
	paused := false
	row, button, _, _ := InfoRow(&InfoRowOptions{
		CaptureText:  "Flow Capture",
		LogLevelText: "Log level: info",
		ShowLogLevel: true,
		DurationText: "Duration: 3s",
		CaptureSize:  "Capture size: 12KB",
		OnPause: func() {
			paused = true
		},
	})

	output := renderPrimitive(t, row)
	assert.Contains(t, output, "Flow Capture")
	assert.Contains(t, output, "Log level:")
	assert.Contains(t, output, "info")
	assert.Contains(t, output, "Duration: 3s")
	assert.Contains(t, output, "Capture size:")
	assert.Contains(t, output, "12KB")

	press(t, button, tcell.KeyEnter)
	assert.True(t, paused)
}

func TestCountRowRendersAndUpdatesThroughButtons(t *testing.T) {
	value := 2
	changes := []int{}
	row := CountRow(CountRowOptions{
		Text:      func(value int) string { return "Showing " + string(rune('0'+value)) },
		Value:     value,
		Minimum:   1,
		Step:      1,
		OnChange:  func(value int) { changes = append(changes, value) },
		UseSpacer: true,
	})

	assert.Contains(t, renderPrimitive(t, row), "Showing 2")
	minus := row.GetItem(1).(*tview.Button)
	plus := row.GetItem(2).(*tview.Button)
	press(t, minus, tcell.KeyEnter)
	press(t, minus, tcell.KeyEnter)
	press(t, plus, tcell.KeyEnter)

	assert.Equal(t, []int{1, 2}, changes)
	assert.Contains(t, renderPrimitive(t, row), "Showing 2")
}

func TestFilterInputAddsAndRemovesFilters(t *testing.T) {
	filters := []string{}
	refreshes := 0
	input := FilterInput(FilterInputOptions{
		CanRemove: func() bool { return len(filters) > 0 },
		RemoveLast: func() {
			filters = filters[:len(filters)-1]
		},
		Add:     func(filter string) { filters = append(filters, filter) },
		Refresh: func() { refreshes++ },
	})

	input.SetText("proto=tcp")
	press(t, input, tcell.KeyEnter)
	assert.Equal(t, []string{"proto=tcp"}, filters)
	assert.Equal(t, "", input.GetText())

	press(t, input, tcell.KeyBackspace)
	assert.Empty(t, filters)
	assert.Equal(t, 2, refreshes)
}

func TestFilterViewRendersAndRemovesSelectedFilter(t *testing.T) {
	filters := []string{"proto=tcp", "port=8080"}
	removed := -1
	view := FilterView(FilterViewOptions{
		Filters: filters,
		OnRemove: func(index int) {
			removed = index
		},
	})

	output := renderPrimitive(t, view)
	assert.Contains(t, output, "Current filters:")
	assert.Contains(t, output, "proto=tcp")
	assert.Contains(t, output, "port=8080")

	press(t, view.GetItem(1), tcell.KeyEnter)
	assert.Equal(t, 0, removed)
}

func TestTableViewRendersAndReportsSelection(t *testing.T) {
	data := NewTableData()
	data.SetData([]string{"SrcAddr", "DstAddr"}, []config.GenericMap{{
		"SrcAddr": "10.0.0.1",
		"DstAddr": "10.0.0.2",
	}})
	selectedRow, selectedColumn := -1, -1
	table := TableView(&TableViewOptions{
		Data:  data,
		Title: func() string { return "Flows" },
		OnSelectionChanged: func(row, column int) {
			selectedRow, selectedColumn = row, column
		},
		OnSelected: func(row, column int) {
			selectedRow, selectedColumn = row, column
		},
	})

	output := renderPrimitive(t, table)
	assert.Contains(t, output, "Flows")
	assert.Contains(t, output, "10.0.0.1")
	assert.Contains(t, output, "10.0.0.2")

	table.Select(1, 1)
	assert.Equal(t, 1, selectedRow)
	assert.Equal(t, 1, selectedColumn)
	press(t, table, tcell.KeyEnter)
	assert.Equal(t, 1, selectedRow)
	assert.Equal(t, 1, selectedColumn)
}

func TestPayloadAndLegendRenderContent(t *testing.T) {
	payload := PayloadPanel(PayloadOptions{
		Data:  []byte("GET /health HTTP/1.1\r\n\r\n"),
		Kind:  PayloadWire,
		Title: "Wire payload",
	})
	assert.Contains(t, renderPrimitive(t, payload), "GET /health")

	legend := Legends("Traffic", []string{"zone"}, []map[string]string{{"zone": "worker"}}, [][]float64{{12.5}}, []tcell.Color{tcell.ColorRed})
	output := renderPrimitive(t, legend)
	assert.Contains(t, output, "Traffic legend")
	assert.Contains(t, output, "worker")
	assert.Contains(t, output, "12.50")
}

func TestChecklistRendersItems(t *testing.T) {
	selected := map[int]bool{1: true}
	modal := ChecklistModal(&ChecklistModalOptions{
		Title:       "Manage panels",
		Description: "Select panels",
		Items:       []string{"Flows", "Packets"},
		Width:       70,
		Height:      12,
		Selected:    func(index int) bool { return selected[index] },
		OnToggle:    func(index int) { selected[index] = !selected[index] },
		OnReset:     func() { selected = map[int]bool{} },
		OnSave:      func() {},
	})

	output := renderPrimitive(t, modal)
	require.Contains(t, output, "Manage panels")
	assert.Contains(t, output, "Select panels")
	assert.Contains(t, output, "Flows")
	assert.Contains(t, output, "Packets")
	assert.Contains(t, output, "[ X ]")
	assert.Contains(t, output, "Save")
}
