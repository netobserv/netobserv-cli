package display

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/navidys/tvxwidgets"
	collectorformat "github.com/netobserv/network-observability-cli/internal/collector/format"
	collectormetrics "github.com/netobserv/network-observability-cli/internal/collector/metrics"
	"github.com/rivo/tview"
)

// Graph contains the data rendered by one metrics panel.
type Graph struct {
	Plot *tvxwidgets.Plot

	Query   collectormetrics.Query
	Labels  []string
	Legends []map[string]string
	Data    [][]float64
}

func NewPlot(title string, colors []tcell.Color) *tvxwidgets.Plot {
	plot := tvxwidgets.NewPlot()
	plot.SetBorder(true)
	plot.SetTitle(title)
	plot.SetAxesColor(tcell.ColorWhite)
	plot.SetAxesLabelColor(tcell.ColorWhite)
	plot.SetPlotType(tvxwidgets.PlotTypeScatter)
	plot.SetMarker(tvxwidgets.PlotMarkerBraille)
	plot.SetLineColor(colors)
	return plot
}

func Legends(title string, labels []string, legends []map[string]string, data [][]float64, colors []tcell.Color) tview.Primitive {
	container := tview.NewFlex().SetDirection(tview.FlexRow)
	container.SetBorder(true)
	container.SetTitle(fmt.Sprintf("%s legend", title))
	table := tview.NewTable()
	table.SetCell(0, 0, tview.NewTableCell("   ").SetTextColor(tcell.ColorWhite).SetBackgroundColor(tcell.ColorBlue))
	for i, label := range labels {
		table.SetCell(0, i+1, tview.NewTableCell(collectorformat.EllipsizeAndPad(label, 50)).SetTextColor(tcell.ColorWhite).SetBackgroundColor(tcell.ColorBlue))
	}
	table.SetCell(0, len(labels)+1, tview.NewTableCell("Value").SetTextColor(tcell.ColorWhite).SetBackgroundColor(tcell.ColorBlue))
	for i := range legends {
		color := tcell.ColorWhite
		if i < len(colors) {
			color = colors[i]
		}
		table.SetCell(i+1, 0, tview.NewTableCell("•••").SetTextColor(color))
		for j, label := range labels {
			table.SetCell(i+1, j+1, tview.NewTableCell(collectorformat.EllipsizeAndPad(legends[i][label], 50)))
		}
		if len(data) > i && len(data[i]) > 0 {
			table.SetCell(i+1, len(labels)+1, tview.NewTableCell(collectorformat.EllipsizeAndPad(fmt.Sprintf("%.2f", data[i][len(data[i])-1]), 50)))
		}
	}
	container.AddItem(table, 0, 1, false)
	return container
}
