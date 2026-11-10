package collector

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	collectordisplay "github.com/netobserv/network-observability-cli/internal/collector/display"
	collectorformat "github.com/netobserv/network-observability-cli/internal/collector/format"
	collectormetrics "github.com/netobserv/network-observability-cli/internal/collector/metrics"
)

const hhmmss24h = "15:04:05"

func createMetricDisplay() {
	updateShowMetricCount()

	controller := collectordisplay.NewController(func() tview.Primitive { return getPages() }, handleMetricKey)
	ui.App = controller.Application

	go heartbeat()

	ui.err = controller.Run()
	if ui.err != nil {
		log.Errorf("Can't display advanced UI: %v", ui.err)
	}
}

func handleMetricKey(event *tcell.EventKey) *tcell.EventKey {
	//nolint:exhaustive
	switch event.Key() {
	case tcell.KeyCtrlC:
		log.Info("Ctrl-C pressed, exiting program.")
		if ui.App != nil {
			ui.App.Stop()
		}
		return nil
	case tcell.KeyCtrlSpace:
		pause(!ui.paused)
		return nil
	default:
		return event
	}
}

func getMetricTop() tview.Primitive {
	topView := tview.NewFlex().SetDirection(tview.FlexRow)
	topView.AddItem(getInfoRow(), 1, 0, false)
	countRow := getCountRow(false)
	countRow.AddItem(tview.NewDropDown().
		SetLabel("Time range ").
		SetOptions(durations, nil).
		SetCurrentOption(selectedDuration).
		SetSelectedFunc(func(_ string, index int) {
			selectedDuration = index
			updateShowMetricCount()
			updateGraphs(true)
		}).
		SetFieldWidth(5), 16, 0, false)
	topView.AddItem(countRow, 1, 0, false)

	panelsRow := tview.NewFlex().SetDirection(tview.FlexColumn)
	panelsRow.AddItem(ui.PanelsTextView, 0, 1, false)
	if len(ui.selectedPanels) == 0 {
		panelsRow.
			AddItem(tview.NewButton("←").SetSelectedFunc(func() {
				panels.Prev()
				updatePanels(true)
				updateScreen()
			}), 5, 0, false).
			AddItem(tview.NewButton("→").SetSelectedFunc(func() {
				panels.Next()
				updatePanels(true)
				updateScreen()
			}), 5, 0, false)
	} else {
		panelsRow.AddItem(tview.NewButton("⟲").SetSelectedFunc(func() {
			ui.selectedPanels = []string{}
			panels.Current = defaultPanelsIndex
			updatePanels(true)
			updateScreen()
		}), 10, 0, false)
	}
	panelsRow.AddItem(tview.NewTextView(), 0, 2, false)
	if !ui.paused {
		updatePanels(false)
	}
	panelsRow.AddItem(tview.NewButton(" Manage panels ").SetSelectedFunc(func() {
		ui.showPopup = true
		ui.App.SetRoot(getPages(), true)
	}), 16, 0, false)
	topView.AddItem(panelsRow, 1, 0, false)
	return topView
}

func getMetricMain() tview.Primitive {
	ui.MainView = tview.NewFlex().SetDirection(tview.FlexRow)
	ui.MainView.AddItem(getMetricTop(), 3, 0, false)
	ui.MainView.AddItem(getGraphs(), 0, 1, false)
	return ui.MainView
}

func updateShowMetricCount() {
	ui.showCount = metricCounts[durations[selectedDuration]]
	ui.CountTextView.SetText(getShowCountText())
}

func getMetricShowCountText() string {
	return fmt.Sprintf("Showing %d points per graph", ui.showCount)
}

func getPanelsText() string {
	if len(ui.selectedPanels) > 0 {
		return "Custom panels"
	}
	return fmt.Sprintf("Display: %s\n", panels.CurrentItem().Name)
}

func getMetricsModal() tview.Primitive {
	availablePanels := []string{}
	for _, panel := range panels.All {
		availablePanels = append(availablePanels, panel.IDs...)
	}
	return collectordisplay.ChecklistModal(&collectordisplay.ChecklistModalOptions{
		Title: "Manage panels", Description: "Highlight a panel and select / unselect it pressing the `Enter` key.",
		Items: availablePanels, Width: 150, Height: 30,
		Label:    func(panel string) string { return collectorformat.ToMetricName(panel, 140) },
		Selected: func(index int) bool { return slices.Contains(ui.selectedPanels, availablePanels[index]) },
		OnToggle: func(index int) {
			panel := availablePanels[index]
			if selected := slices.Index(ui.selectedPanels, panel); selected >= 0 {
				ui.selectedPanels = slices.Delete(ui.selectedPanels, selected, selected+1)
			} else {
				ui.selectedPanels = append(ui.selectedPanels, panel)
			}
			updatePanels(true)
		},
		OnReset: func() { ui.selectedPanels = nil; updatePanels(false) },
		OnSave:  func() { updateGraphs(true); updateScreen() },
	})
}

func updateGraphs(query bool) {
	ui.Graphs = make([]collectordisplay.Graph, 0)
	ui.focusedGraph = -1

	panelIDs := ui.selectedPanels
	if len(panelIDs) == 0 {
		panelIDs = panels.CurrentItem().IDs
	}
	for _, panel := range panelIDs {
		ui.Graphs = append(ui.Graphs, collectordisplay.Graph{Query: collectormetrics.Query{PromQL: panel}})
	}

	getGraphs()
	if query && client != nil {
		go queryGraphs(context.TODO(), *client)
	}
}

func updatePanels(query bool) {
	ui.PanelsTextView.SetText(getPanelsText())
	updateGraphs(query)
}

func appendMetrics(query *collectormetrics.Query, matrix *collectormetrics.Matrix, index int) {
	if ui.paused || query == nil || matrix == nil || index >= len(ui.Graphs) || ui.Graphs[index].Query.PromQL != query.PromQL {
		return
	}

	ui.Graphs[index].Query = *query
	updateData(matrix, index)
}

func updateData(matrix *collectormetrics.Matrix, index int) {
	labels, legends, data := collectormetrics.MatrixData(*matrix, len(ui.Colors))
	ui.Graphs[index].Labels = labels
	ui.Graphs[index].Legends = legends
	ui.Graphs[index].Data = data
}

func getGraphs() tview.Primitive {
	ui.GraphsContainer.Clear()

	var flex *tview.Flex
	for index := range ui.Graphs {
		plot := collectordisplay.NewPlot(collectorformat.ToMetricName(ui.Graphs[index].Query.PromQL, 0), ui.Colors)
		if ui.focusedGraph == index {
			plot.SetBorderColor(tcell.ColorBlue)
		}
		ui.Graphs[index].Plot = plot

		if index%2 == 0 {
			flex = tview.NewFlex().SetDirection(tview.FlexColumn)
			flex.SetRect(0, 0, 100, 15)
			ui.GraphsContainer.AddItem(flex, 0, 1, false)
		}
		flex.AddItem(plot, 0, 1, false)
	}
	return ui.GraphsContainer
}

func getLegends(title string, labels []string, legends []map[string]string, data [][]float64) tview.Primitive {
	ui.LegendView = collectordisplay.Legends(title, labels, legends, data, ui.Colors).(*tview.Flex)
	return ui.LegendView
}

func updatePlots() {
	for index := range ui.Graphs {
		graph := &ui.Graphs[index]
		if graph.Plot == nil {
			continue
		}
		graph.Plot.SetXAxisLabelFunc(func(i int) string {
			return graph.Query.Range.Start.Add(time.Duration(i) * graph.Query.Range.Step).Format(hhmmss24h)
		})

		if len(graph.Labels) > 0 {
			graph.Plot.SetBlurFunc(func() {
				ui.focusedGraph = -1
				if ui.LegendView != nil {
					ui.MainView.RemoveItem(ui.LegendView)
					getGraphs()
				}
			})
			graph.Plot.SetFocusFunc(func() {
				ui.focusedGraph = index
				getGraphs()
				ui.MainView.AddItem(getLegends(graph.Query.PromQL, graph.Labels, graph.Legends, graph.Data), len(graph.Legends)+3, 0, false)
			})
		} else {
			graph.Plot.SetFocusFunc(func() { getGraphs() })
		}

		if graph.Data != nil {
			graph.Plot.SetData(graph.Data)
		}
	}
}
