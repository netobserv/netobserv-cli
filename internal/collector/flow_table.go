package collector

import (
	"encoding/base64"
	"fmt"
	"slices"

	"github.com/netobserv/flowlogs-pipeline/pkg/config"
	collectordisplay "github.com/netobserv/network-observability-cli/internal/collector/display"
	collectorflow "github.com/netobserv/network-observability-cli/internal/collector/flow"
	collectorformat "github.com/netobserv/network-observability-cli/internal/collector/format"
	collectorpacket "github.com/netobserv/network-observability-cli/internal/collector/packet"
	"github.com/rivo/tview"
)

func getColumnsModal() tview.Primitive {
	availableColumns := []*collectordisplay.ColumnConfig{}
	for _, col := range collectordisplay.CurrentConfig().Columns {
		if col.Field == "" || (col.Feature == plaintextCaptureFeature && (capture != Packet || !plaintextCaptureEnabled())) {
			continue
		}
		availableColumns = append(availableColumns, col)
	}

	items := make([]string, len(availableColumns))
	for i, column := range availableColumns {
		items[i] = column.ID
	}
	return collectordisplay.ChecklistModal(&collectordisplay.ChecklistModalOptions{
		Title: "Manage columns", Description: "Highlight a column and select / unselect it pressing the `Enter` key.",
		Items: items, Width: 50, Height: 30,
		Label:    func(id string) string { return collectorformat.ToColName(id, 40) },
		Selected: func(index int) bool { return slices.Contains(ui.selectedColumns, availableColumns[index].ID) },
		OnToggle: func(index int) {
			id := availableColumns[index].ID
			if selected := slices.Index(ui.selectedColumns, id); selected >= 0 {
				ui.selectedColumns = slices.Delete(ui.selectedColumns, selected, selected+1)
			} else {
				ui.selectedColumns = append(ui.selectedColumns, id)
			}
			updateDisplayEnrichmentTexts()
		},
		OnDefaults: func() {
			ui.selectedColumns = nil
			for _, column := range availableColumns {
				if column.Default {
					ui.selectedColumns = append(ui.selectedColumns, column.ID)
				}
			}
			updateDisplayEnrichmentTexts()
		},
		OnReset: func() { ui.selectedColumns = nil; updateDisplayEnrichmentTexts() },
		OnSave:  updateScreen,
	})
}

func getTable() *tview.Table {
	if ui.TableView != nil {
		ui.TableView.SetTitle(getTableTitle())
		return ui.TableView
	}

	ui.TableView = collectordisplay.TableView(&collectordisplay.TableViewOptions{
		Data:              ui.TableData,
		Title:             getTableTitle,
		SuppressSelection: func() bool { return ui.suppressTableSelection },
		OnSelectionChanged: func(row, column int) {
			ui.focus = "table"
			ui.selectedTableColumn = column
			index := row - 1
			if row <= 0 || index >= len(ui.TableData.Flows()) {
				resetSelection()
				return
			}
			selectedFlow := ui.TableData.Flows()[index]
			if idx, ok := selectedFlow["Index"].(int); ok {
				ui.selectedFlowIndex = idx
			}
			if collectorpacket.IsPlaintextRecord(selectedFlow) {
				payload := collectorformat.PlaintextRawBytes(selectedFlow)
				if len(payload) == 0 {
					if summary := collectorformat.PlaintextBinarySummary(selectedFlow); summary != "" {
						selectData([]byte(summary), payloadKindPlaintext)
					}
					return
				}
				selectData(payload, payloadKindPlaintext)
				return
			}
			data, ok := selectedFlow["Data"]
			if ok {
				bytes, err := base64.StdEncoding.DecodeString(data.(string))
				if err != nil {
					log.Error("Error while decoding data", err)
					return
				}
				selectData(bytes, payloadKindWire)
			}
		},
		OnSelected: func(row, col int) {
			if row <= 0 || ui.InputField == nil {
				return
			}
			id := ui.TableData.Columns()[col]
			index := row - 1
			if index < len(ui.TableData.Flows()) {
				fieldName := collectorformat.ToFieldName(id)
				value, ok := ui.TableData.Flows()[index][fieldName]
				if !ok || value == nil {
					return
				}
				ui.focus = "inputField"
				updateScreen()
				ui.InputField.SetText(fmt.Sprintf("%s:%v", fieldName, value))
			}
		},
	})

	return ui.TableView
}

func getCols() []string {
	cols := []string{}
	if len(ui.selectedColumns) > 0 {
		cols = ui.selectedColumns
	} else if display.CurrentItem().Name == rawDisplay {
		cols = append(cols, rawDisplay)
	} else {
		cols = append(cols, "EndTime")
		if enrichment.CurrentItem().Name != noOptions {
			cols = append(cols, enrichment.CurrentItem().IDs...)
		} else {
			cols = append(cols, "SrcAddr", "SrcPort", "DstAddr", "DstPort")
		}
		cols = append(cols, "Interfaces", "IfDirections")
		if display.CurrentItem().Name != standardDisplay {
			for _, col := range collectordisplay.CurrentConfig().Columns {
				if col.Field != "" && slices.Contains(display.CurrentItem().IDs, col.Feature) {
					cols = append(cols, col.ID)
				}
			}
		} else {
			cols = append(cols, "FlowDirection", "Proto", "Dscp", "Bytes", "Packets")
		}
	}
	return collectordisplay.AddPlaintextColumns(cols, collectordisplay.CurrentConfig(), capture == Packet, plaintextCaptureEnabled(), ui.selectedColumns)
}

func getFlows() []config.GenericMap {
	return collectorflow.Visible(packetCaptureFlowSnapshot(), ui.TableData.Flows(), ui.regexes, ui.showCount, capture == Packet)
}

func updateTableAndSuggestions() {
	ui.TableData.SetData(getCols(), getFlows())
	ui.suggestions = []string{}
	for _, flow := range ui.TableData.Flows() {
		for key, value := range flow {
			if !slices.Contains(ui.suggestions, key) {
				ui.suggestions = append(ui.suggestions, key)
			}
			valueString := fmt.Sprintf("%v", value)
			if !slices.Contains(ui.suggestions, valueString) {
				ui.suggestions = append(ui.suggestions, valueString)
			}
		}
	}
	restoreTableSelection()
}

func selectData(data []byte, kind string) {
	ui.selectedData = data
	ui.selectedPayloadKind = kind
	pause(true)
}

func resetSelection() {
	ui.selectedData = []byte{}
	ui.selectedPayloadKind = ""
	ui.selectedFlowIndex = -1
	pause(false)
}

func getPayloadPanel() tview.Primitive {
	return collectordisplay.PayloadPanel(collectordisplay.PayloadOptions{
		Data: ui.selectedData, Kind: ui.selectedPayloadKind, Title: hexPanelTitle(),
	})
}

func hexPanelTitle() string {
	if capture != Packet {
		return "Payload"
	}
	switch ui.selectedPayloadKind {
	case payloadKindPlaintext:
		return "TLS Plaintext"
	case payloadKindWire:
		return collectorformat.WirePayloadPanelTitle(ui.selectedData)
	default:
		return "Payload"
	}
}

func restoreTableSelection() {
	if ui.TableView == nil || ui.selectedFlowIndex < 0 || len(ui.TableData.Columns()) == 0 {
		return
	}
	for i, f := range ui.TableData.Flows() {
		if idx, ok := f["Index"].(int); ok && idx == ui.selectedFlowIndex {
			column := ui.selectedTableColumn
			if column < 0 {
				column = 0
			}
			if column >= len(ui.TableData.Columns()) {
				column = len(ui.TableData.Columns()) - 1
			}
			ui.suppressTableSelection = true
			ui.TableView.Select(i+1, column)
			ui.suppressTableSelection = false
			return
		}
	}
}
