package display

import (
	"github.com/gdamore/tcell/v2"
	"github.com/netobserv/flowlogs-pipeline/pkg/config"
	collectorformat "github.com/netobserv/network-observability-cli/internal/collector/format"
	"github.com/rivo/tview"
)

// TableData adapts captured records to tview's table model.
type TableData struct {
	cols  []string
	flows []config.GenericMap
	tview.TableContentReadOnly
}

func NewTableData() *TableData { return &TableData{} }

func (d *TableData) SetData(cols []string, flows []config.GenericMap) {
	d.cols, d.flows = cols, flows
}

func (d *TableData) Columns() []string          { return d.cols }
func (d *TableData) Flows() []config.GenericMap { return d.flows }
func (d *TableData) GetRowCount() int           { return len(d.flows) + 1 }
func (d *TableData) GetColumnCount() int        { return len(d.cols) }

func (d *TableData) Rows() []string {
	if len(d.cols) == 0 || len(d.flows) == 0 {
		return nil
	}
	rows := make([]string, 0, len(d.flows)+1)
	for row := 0; row < len(d.flows)+1; row++ {
		var text string
		for col := range d.cols {
			text += d.GetCell(row, col).Text
		}
		rows = append(rows, text)
	}
	return rows
}

func (d *TableData) GetCell(row, col int) *tview.TableCell {
	if len(d.cols) == 0 {
		return tview.NewTableCell("Initializing...")
	}
	if row < 0 {
		return tview.NewTableCell("invalid row")
	}
	if col < 0 || col >= len(d.cols) {
		return tview.NewTableCell("invalid col")
	}

	id := d.cols[col]
	width := collectorformat.ToColWidth(id)
	color, background := tcell.ColorWhite, tcell.ColorBlack
	if row == 0 {
		background = tcell.ColorBlue
	} else if col == 0 {
		color = tcell.ColorYellow
	} else if id == "EndTime" {
		color, background = tcell.ColorYellow, tcell.ColorWhite
	}
	if row == 0 {
		return tview.NewTableCell(collectorformat.ToColName(id, width)).
			SetTextColor(color).SetBackgroundColor(background).
			SetAlign(tview.AlignLeft).SetMaxWidth(width)
	}

	index := row - 1
	if index >= len(d.flows) {
		return tview.NewTableCell("")
	}
	cellColor := color
	if collectorformat.IsPlaintextRecord(d.flows[index]) {
		cellColor = tcell.ColorLightGreen
	}
	return tview.NewTableCell(collectorformat.ToColValue(d.flows[index], id, width)).
		SetTextColor(cellColor).SetBackgroundColor(background).
		SetAlign(tview.AlignLeft).SetMaxWidth(width)
}

type TableViewOptions struct {
	Data               *TableData
	Title              func() string
	SuppressSelection  func() bool
	OnSelectionChanged func(row, column int)
	OnSelected         func(row, column int)
}

// TableView creates the flow table and delegates domain-specific selection
// behavior to the caller.
func TableView(options *TableViewOptions) *tview.Table {
	table := tview.NewTable().
		SetBorders(false).
		SetSelectable(true, true).
		SetSelectionChangedFunc(func(row, column int) {
			if options.SuppressSelection != nil && options.SuppressSelection() {
				return
			}
			if options.OnSelectionChanged != nil {
				options.OnSelectionChanged(row, column)
			}
		}).
		SetSelectedFunc(func(row, column int) {
			if options.OnSelected != nil {
				options.OnSelected(row, column)
			}
		}).
		SetContent(options.Data)
	table.SetBorder(true).SetTitle(options.Title())
	return table
}
