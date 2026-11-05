package collector

import (
	"fmt"
	"slices"

	"github.com/gdamore/tcell/v2"
	// Package extended contains an extended set of terminal descriptions
	// to have a better chance of working by default
	_ "github.com/gdamore/tcell/v2/terminfo/extended"
	"github.com/rivo/tview"

	collectordisplay "github.com/netobserv/network-observability-cli/internal/collector/display"
)

const (
	defaultFlowShowCount = 30 // flows to display
	defaultExtraWidth    = 5  // additionnal column width

	payloadKindWire      = "wire"
	payloadKindPlaintext = "plaintext"
)

func createFlowDisplay() {
	ui.focus = "inputField"
	controller := collectordisplay.NewController(func() tview.Primitive { return getPages() }, handleFlowKey)
	ui.App = controller.Application

	go heartbeat()

	ui.err = controller.Run()
	if ui.err != nil {
		log.Errorf("Can't display advanced UI: %v", ui.err)
	}
}

func handleFlowKey(event *tcell.EventKey) *tcell.EventKey {
	//nolint:exhaustive
	switch event.Key() {
	case tcell.KeyCtrlC:
		log.Info("Ctrl-C pressed, exiting program.")
		if ui.App != nil {
			ui.App.Stop()
		}
		return nil
	case tcell.KeyESC:
		resetSelection()
		return nil
	case tcell.KeyTab:
		if ui.focus == "inputField" {
			ui.focus = "table"
		} else if ui.focus == "table" && ui.paused && len(ui.selectedData) > 0 {
			ui.focus = "hex"
		} else {
			ui.focus = "inputField"
		}
		updateScreen()
		return nil
	case tcell.KeyCtrlD:
		display.Next()
		updateDisplayEnrichmentTexts()
		updateScreen()
		return nil
	case tcell.KeyCtrlE:
		enrichment.Next()
		updateDisplayEnrichmentTexts()
		updateScreen()
		return nil
	case tcell.KeyCtrlSpace:
		pause(!ui.paused)
		return nil
	default:
		return event
	}
}

func getFilters() *tview.Flex {
	ui.FiltersView = collectordisplay.FilterView(collectordisplay.FilterViewOptions{
		Filters: ui.regexes,
		OnRemove: func(index int) {
			ui.regexes = slices.Delete(ui.regexes, index, index+1)
			updateScreen()
		},
	})
	return ui.FiltersView
}

func getBottom() tview.Primitive {
	flexView := tview.NewFlex().SetDirection(tview.FlexColumn)

	if ui.InputField == nil {
		ui.InputField = collectordisplay.FilterInput(collectordisplay.FilterInputOptions{
			Suggestions: func() []string { return ui.suggestions },
			CanRemove:   func() bool { return len(ui.regexes) > 0 },
			RemoveLast:  func() { ui.regexes = ui.regexes[:len(ui.regexes)-1] },
			Add:         func(filter string) { ui.regexes = append(ui.regexes, filter) },
			Refresh:     func() { ui.FiltersView = getFilters(); updateScreen() },
		})
	}
	flexView.AddItem(ui.InputField, 51, 0, ui.focus == "inputField")
	flexView.AddItem(getFilters(), 0, 1, false)

	return flexView
}

func getFlowMain() tview.Primitive {
	ui.MainView = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(getFlowTop(), 4, 0, false)

	ui.MainView.AddItem(getTable(), 0, 1, ui.focus == "table")

	if ui.paused {
		if len(ui.selectedData) > 0 {
			ui.MainView.AddItem(getPayloadPanel(), 0, 1, ui.focus == "hex")
		}
	}
	ui.MainView.AddItem(getBottom(), 1, 0, ui.focus == "inputField")
	return ui.MainView
}

func getFlowTop() tview.Primitive {
	flexView := tview.NewFlex().SetDirection(tview.FlexRow)

	// info row
	flexView.AddItem(getInfoRow(), 1, 0, false)

	// flows count
	flexView.AddItem(getCountRow(true), 1, 0, false)

	// columns row containing cycles (display, enrichment) and custom columns picker
	columnsRow := tview.NewFlex().SetDirection(tview.FlexColumn)
	cyclesCol := tview.NewFlex().SetDirection(tview.FlexRow)

	// display
	displayRow := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(ui.DisplayTextView, 0, 1, false)
	if len(ui.selectedColumns) == 0 {
		displayRow.
			AddItem(tview.NewButton("←").SetSelectedFunc(func() {
				display.Prev()
				updateDisplayEnrichmentTexts()
				updateScreen()
			}), 5, 0, false).
			AddItem(tview.NewButton("→").SetSelectedFunc(func() {
				display.Next()
				updateDisplayEnrichmentTexts()
				updateScreen()
			}), 5, 0, false)
	} else {
		displayRow.
			AddItem(tview.NewButton("⟲").SetSelectedFunc(func() {
				ui.selectedColumns = []string{}
				display.Current = defaultDisplayIndex
				updateDisplayEnrichmentTexts()
				updateScreen()
			}), 10, 0, false)
	}
	displayRow.AddItem(tview.NewTextView(), 0, 2, false)
	cyclesCol.AddItem(displayRow, 0, 1, false)

	// enrichment
	enrichmentRow := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(ui.EnrichmentTextView, 0, 1, false)
	if display.CurrentItem().Name != rawDisplay && len(ui.selectedColumns) == 0 {
		enrichmentRow.
			AddItem(tview.NewButton("←").SetSelectedFunc(func() {
				enrichment.Prev()
				updateDisplayEnrichmentTexts()
				updateScreen()
			}), 5, 0, false).
			AddItem(tview.NewButton("→").SetSelectedFunc(func() {
				enrichment.Next()
				updateDisplayEnrichmentTexts()
				updateScreen()
			}), 5, 0, false)
	}
	enrichmentRow.AddItem(tview.NewTextView(), 0, 2, false)
	cyclesCol.AddItem(enrichmentRow, 0, 1, false)
	updateDisplayEnrichmentTexts()

	// add cycles and custom columns modal button
	columnsRow.AddItem(cyclesCol, 0, 1, false)
	columnsRow.AddItem(tview.NewButton(" Manage columns ").SetSelectedFunc(func() {
		ui.showPopup = true
		ui.App.SetRoot(getPages(), true)
	}), 16, 0, false)
	flexView.AddItem(columnsRow, 2, 0, false)

	return flexView
}

func getCaptureText() string {
	return fmt.Sprintf("%s Capture", capture)
}

func getTableTitle() string {
	if ui.paused {
		return "Table refresh is paused. Press `ESC` to resume."
	}
	return "Flows"
}

func getLogLevelText() string {
	return fmt.Sprintf("Log level: %s", logLevel)
}

func getEnrichmentText() string {
	if len(ui.selectedColumns) > 0 {
		return ""
	} else if display.CurrentItem().Name == rawDisplay {
		return "Enrichment: n/a\n"
	}
	return fmt.Sprintf("Enrichment: %s\n", enrichment.CurrentItem().Name)
}

func getDisplayText() string {
	if len(ui.selectedColumns) > 0 {
		return "Custom columns"
	}
	return fmt.Sprintf("Display: %s\n", display.CurrentItem().Name)
}

func getFlowShowCountText() string {
	return fmt.Sprintf("Showing last: %d\n", ui.showCount)
}

func updateDisplayEnrichmentTexts() {
	ui.DisplayTextView.SetText(getDisplayText())
	ui.EnrichmentTextView.SetText(getEnrichmentText())
}
