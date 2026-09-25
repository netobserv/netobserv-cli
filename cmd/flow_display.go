package cmd

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	hexview "github.com/jmhobbs/tview-hexview"
	"github.com/netobserv/flowlogs-pipeline/pkg/config"

	"github.com/gdamore/tcell/v2"
	// Package extended contains an extended set of terminal descriptions
	// to have a better chance of working by default
	_ "github.com/gdamore/tcell/v2/terminfo/extended"
	"github.com/rivo/tview"
)

type TableData struct {
	cols  []string
	flows []config.GenericMap
	tview.TableContentReadOnly
}

var keepCount = defaultKeepCount // flows/rows kept in memory (general buffer)

const (
	defaultKeepCount           = 100 // flow rows, and non-TLS packet-capture rows, kept in memory
	defaultWirePacketKeepCount = 200 // wire PCA rows when TLS capture is on
	defaultPlaintextKeepCount  = 300 // TLS plaintext rows when TLS capture is on
	defaultFlowShowCount       = 30  // flows to display
	defaultExtraWidth          = 5   // additionnal column width

	payloadKindWire      = "wire"
	payloadKindPlaintext = "plaintext"
)

var (
	flowIndex          = 0
	regexes            = []string{}
	lastFlows          = []config.GenericMap{}
	lastWirePackets    = []config.GenericMap{}
	lastPlaintextFlows = []config.GenericMap{}
	suggestions        = []string{}
	selectedColumns    = []string{}

	extraWidth = defaultExtraWidth

	tableView   *tview.Table
	filtersView *tview.Flex

	displayTextView    = tview.NewTextView()
	enrichmentTextView = tview.NewTextView()

	inputField *tview.InputField

	tableData = &TableData{
		cols:  []string{},
		flows: []config.GenericMap{},
	}
	selectedData        = []byte{}
	selectedPayloadKind = ""
	selectedFlowIndex   = -1

	suppressTableSelection bool
)

func createFlowDisplay() {
	focus = "inputField"
	app = tview.NewApplication().
		SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			//nolint:exhaustive
			switch event.Key() {
			case tcell.KeyCtrlC:
				log.Info("Ctrl-C pressed, exiting program.")
				if app != nil {
					app.Stop()
				}
			case tcell.KeyESC:
				// reset pages when esc key pressed
				resetSelection()
			case tcell.KeyTab:
				// focus on table, hex viewer if available and input field
				if focus == "inputField" {
					focus = "table"
				} else if focus == "table" && paused && len(selectedData) > 0 {
					focus = "hex"
				} else {
					focus = "inputField"
				}
				updateScreen()
			case tcell.KeyCtrlD:
				display.next()
				updateDisplayEnrichmentTexts()
				updateScreen()
			case tcell.KeyCtrlE:
				enrichment.next()
				updateDisplayEnrichmentTexts()
				updateScreen()
			case tcell.KeyCtrlSpace:
				pause(!paused)
			default:
				// nothing to do here
			}
			return event
		}).
		SetRoot(getPages(), true).
		EnableMouse(true)

	go hearbeat()

	errAdvancedDisplay = app.Run()
	if errAdvancedDisplay != nil {
		log.Errorf("Can't display advanced UI: %v", errAdvancedDisplay)
	}
}

func getFlowMain() tview.Primitive {
	mainView = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(getFlowTop(), 4, 0, false)

	mainView.AddItem(getTable(), 0, 1, focus == "table")

	if paused {
		if len(selectedData) > 0 {
			mainView.AddItem(getPayloadPanel(), 0, 1, focus == "hex")
		}
	}
	mainView.AddItem(getBottom(), 1, 0, focus == "inputField")
	return mainView
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
		AddItem(displayTextView, 0, 1, false)
	if len(selectedColumns) == 0 {
		displayRow.
			AddItem(tview.NewButton("←").SetSelectedFunc(func() {
				display.prev()
				updateDisplayEnrichmentTexts()
				updateScreen()
			}), 5, 0, false).
			AddItem(tview.NewButton("→").SetSelectedFunc(func() {
				display.next()
				updateDisplayEnrichmentTexts()
				updateScreen()
			}), 5, 0, false)
	} else {
		displayRow.
			AddItem(tview.NewButton("⟲").SetSelectedFunc(func() {
				selectedColumns = []string{}
				display.current = defaultDisplayIndex
				updateDisplayEnrichmentTexts()
				updateScreen()
			}), 10, 0, false)
	}
	displayRow.AddItem(tview.NewTextView(), 0, 2, false)
	cyclesCol.AddItem(displayRow, 0, 1, false)

	// enrichment
	enrichmentRow := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(enrichmentTextView, 0, 1, false)
	if display.getCurrentItem().name != rawDisplay && len(selectedColumns) == 0 {
		enrichmentRow.
			AddItem(tview.NewButton("←").SetSelectedFunc(func() {
				enrichment.prev()
				updateDisplayEnrichmentTexts()
				updateScreen()
			}), 5, 0, false).
			AddItem(tview.NewButton("→").SetSelectedFunc(func() {
				enrichment.next()
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
		showPopup = true
		app.SetRoot(getPages(), true)
	}), 16, 0, false)
	flexView.AddItem(columnsRow, 2, 0, false)

	return flexView
}

func getTable() *tview.Table {
	if tableView != nil {
		tableView.SetTitle(getTableTitle())
		return tableView
	}

	tableView = tview.NewTable().
		SetBorders(false).
		SetSelectable(true, true).
		SetSelectionChangedFunc(func(row, _ int) {
			if suppressTableSelection {
				return
			}
			focus = "table"
			index := row - 1
			if row <= 0 || index >= len(tableData.flows) {
				resetSelection()
				return
			}
			selectedFlow := tableData.flows[index]
			if idx, ok := selectedFlow["Index"].(int); ok {
				selectedFlowIndex = idx
			}
			if isPlaintextRecord(selectedFlow) {
				payload := plaintextRawBytes(selectedFlow)
				if len(payload) == 0 {
					if summary := plaintextBinarySummary(selectedFlow); summary != "" {
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
		}).
		SetSelectedFunc(func(row, col int) {
			if row <= 0 || inputField == nil {
				return
			}

			id := tableData.cols[col]
			index := row - 1
			if index < len(tableData.flows) {
				fieldName := toFieldName(id)
				value, ok := tableData.flows[index][fieldName]
				if !ok || value == nil {
					return
				}
				focus = "inputField"
				updateScreen()
				inputField.SetText(fmt.Sprintf("%s:%v", fieldName, value))
			}
		}).
		SetContent(tableData)
	tableView.SetBorder(true).SetTitle(getTableTitle())

	return tableView
}

func getFilters() *tview.Flex {
	filtersView = tview.NewFlex().SetDirection(tview.FlexColumn)

	if len(regexes) > 0 {
		filtersView.AddItem(tview.NewTextView().SetText("Current filters:"), 17, 0, false)
		for _, regex := range regexes {
			filtersView.AddItem(tview.NewButton(regex).SetSelectedFunc(func() {
				for i, v := range regexes {
					if v == regex {
						regexes = slices.Delete(regexes, i, i+1)
						updateScreen()
						break
					}
				}
			}), len(regex), 0, false)
			filtersView.AddItem(tview.NewTextView(), 1, 0, false)
		}
		filtersView.AddItem(tview.NewTextView().SetText("Press `Enter` key to add a new one and backspace to remove last one"), 0, 1, false)
	} else {
		filtersView.AddItem(tview.NewTextView().SetText("Press `Enter` key to match multiple regexes at once"), 0, 1, false)
	}

	return filtersView
}

func getBottom() tview.Primitive {
	flexView := tview.NewFlex().SetDirection(tview.FlexColumn)

	if inputField == nil {
		inputField = tview.NewInputField().
			SetLabel("Live table regexes: ").
			SetFieldWidth(30)
		inputField.SetAutocompleteFunc(func(currentText string) (entries []string) {
			if len(currentText) == 0 {
				return
			}
			for _, word := range suggestions {
				if strings.HasPrefix(strings.ToLower(word), strings.ToLower(currentText)) {
					entries = append(entries, word)
				}
			}
			if len(entries) <= 1 {
				entries = nil
			}
			return
		})
		// on any input event
		inputField.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			//nolint:exhaustive
			switch event.Key() {
			case tcell.KeyBackspace, tcell.KeyBackspace2:
				if len(inputField.GetText()) == 0 && len(regexes) > 0 {
					regexes = regexes[:len(regexes)-1]
				}
				filtersView = getFilters()
				updateScreen()
			default:
				// nothing to do here
			}
			return event
		})
		// after input event
		inputField.SetAutocompletedFunc(func(text string, _, source int) bool {
			if source != tview.AutocompletedNavigate {
				inputField.SetText(text)
			}
			return source == tview.AutocompletedEnter || source == tview.AutocompletedClick
		})
		// after autocomplete event
		inputField.SetDoneFunc(func(key tcell.Key) {
			//nolint:exhaustive
			switch key {
			case tcell.KeyEnter:
				text := inputField.GetText()
				if len(text) > 0 {
					regexes = append(regexes, text)
					inputField.SetText("")
				}
				filtersView = getFilters()
				updateScreen()
			default:
				// nothing to do here
			}
		})
	}
	flexView.AddItem(inputField, 51, 0, focus == "inputField")
	flexView.AddItem(getFilters(), 0, 1, false)

	return flexView
}

func getColumnsModal() tview.Primitive {
	availableColumns := []*ColumnConfig{}
	for _, col := range cfg.Columns {
		if col.Field == "" {
			continue
		}
		if col.Feature == plaintextCaptureFeature && (capture != Packet || !plaintextCaptureEnabled()) {
			continue
		}
		availableColumns = append(availableColumns, col)
	}

	content := tview.NewFlex().SetDirection(tview.FlexRow)
	content.SetBorder(true).SetTitle("Manage columns")

	content.AddItem(tview.NewTextView().
		SetText("Highlight a column and select / unselect it pressing the `Enter` key."), 2, 0, false)

	colsTable := tview.NewTable()

	setCell := func(i int, col *ColumnConfig) {
		checkedStr := "[   ]"
		if slices.Contains(selectedColumns, col.ID) {
			checkedStr = "[ X ]"
		}
		colsTable.SetCell(i, 0, tview.NewTableCell(checkedStr))
		colsTable.SetCell(i, 1, tview.NewTableCell(toColName(col.ID, 40)))
	}

	setTableContent := func() {
		for i, col := range availableColumns {
			setCell(i, col)
		}
	}

	onSelect := func(row int) {
		if row < 0 || row >= len(availableColumns) {
			return
		}
		c := availableColumns[row]
		for i, v := range selectedColumns {
			// remove id if found
			if v == c.ID {
				selectedColumns = append(selectedColumns[:i], selectedColumns[i+1:]...)
				setCell(row, c)
				return
			}
		}
		// else add it to selection
		selectedColumns = append(selectedColumns, c.ID)
		setCell(row, c)
		updateDisplayEnrichmentTexts()
	}

	colsTable.SetSelectable(true, false).
		SetSelectedFunc(func(row, _ int) {
			onSelect(row)
		})

	setTableContent()
	content.AddItem(colsTable, 0, 1, true)

	buttons := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(tview.NewButton("Restore defaults").SetSelectedFunc(func() {
			selectedColumns = []string{}
			for i, c := range availableColumns {
				if c.Default {
					selectedColumns = append(selectedColumns, c.ID)
				}
				setCell(i, c)
			}
			updateDisplayEnrichmentTexts()
		}), 0, 1, false).
		AddItem(tview.NewTextView(), 1, 0, false).
		AddItem(tview.NewButton("Reset").SetSelectedFunc(func() {
			selectedColumns = []string{}
			setTableContent()
			updateDisplayEnrichmentTexts()
		}), 0, 1, false).
		AddItem(tview.NewTextView(), 1, 0, false).
		AddItem(tview.NewButton("Save").SetSelectedFunc(func() {
			updateScreen()
		}), 0, 1, false)
	content.AddItem(buttons, 1, 0, false)

	return getModal(content, 50, 30)
}

func getcaptureText() string {
	return fmt.Sprintf("%s Capture", capture)
}

func getTableTitle() string {
	if paused {
		return "Table refresh is paused. Press `ESC` to resume."
	}
	return "Flows"
}

func getLogLevelText() string {
	return fmt.Sprintf("Log level: %s", logLevel)
}

func getEnrichmentText() string {
	if len(selectedColumns) > 0 {
		return ""
	} else if display.getCurrentItem().name == rawDisplay {
		return "Enrichment: n/a\n"
	}
	return fmt.Sprintf("Enrichment: %s\n", enrichment.getCurrentItem().name)
}

func getDisplayText() string {
	if len(selectedColumns) > 0 {
		return "Custom columns"
	}
	return fmt.Sprintf("Display: %s\n", display.getCurrentItem().name)
}

func getFlowShowCountText() string {
	return fmt.Sprintf("Showing last: %d\n", showCount)
}

func getPayloadPanel() tview.Primitive {
	if selectedPayloadKind == payloadKindPlaintext {
		formatted := formatPlaintextPayload(selectedData)
		if formatted != "" {
			text := tview.NewTextView().
				SetDynamicColors(false).
				SetScrollable(true).
				SetWrap(true).
				SetText(formatted)
			text.SetBorder(true).SetTitle(hexPanelTitle())
			return text
		}
		if len(selectedData) > 0 {
			hex := hexview.NewHexView(selectedData)
			hex.SetBorder(true).SetTitle(fmt.Sprintf("TLS Plaintext (binary, %dB)", len(selectedData)))
			return hex
		}
		text := tview.NewTextView().
			SetDynamicColors(false).
			SetText("(empty TLS plaintext payload)")
		text.SetBorder(true).SetTitle(hexPanelTitle())
		return text
	}
	if formatted := formatWireHTTPPayload(selectedData); formatted != "" {
		text := tview.NewTextView().
			SetDynamicColors(false).
			SetScrollable(true).
			SetWrap(true).
			SetText(formatted)
		text.SetBorder(true).SetTitle(wirePayloadPanelTitle(selectedData))
		return text
	}
	hex := hexview.NewHexView(selectedData)
	hex.SetBorder(true).SetTitle(wirePayloadPanelTitle(selectedData))
	return hex
}

func hexPanelTitle() string {
	if capture != Packet {
		return "Payload"
	}
	switch selectedPayloadKind {
	case payloadKindPlaintext:
		return "TLS Plaintext"
	case payloadKindWire:
		return wirePayloadPanelTitle(selectedData)
	default:
		return "Payload"
	}
}

func packetFlowTimeMs(m config.GenericMap) float64 {
	if t := toFloat64(m, "TimeFlowStartMs"); t > 0 {
		return t
	}
	return toFloat64(m, "Time") * 1000
}

func packetFlowIndex(m config.GenericMap) int {
	idx, _ := m["Index"].(int)
	return idx
}

func comparePacketFlows(a, b config.GenericMap) bool {
	ti := packetFlowTimeMs(a)
	tj := packetFlowTimeMs(b)
	if ti != tj {
		return ti < tj
	}
	return packetFlowIndex(a) < packetFlowIndex(b)
}

func sortPacketFlows(flows []config.GenericMap) {
	sort.Slice(flows, func(i, j int) bool {
		return comparePacketFlows(flows[i], flows[j])
	})
}

func isWirePacketRecord(m config.GenericMap) bool {
	_, ok := m["Data"]
	return ok
}

func trimFlowsToShowCount(flows []config.GenericMap, limit int) []config.GenericMap {
	if limit <= 0 {
		return nil
	}
	if len(flows) <= limit {
		return flows
	}
	if capture == Packet {
		sortPacketFlows(flows)
	}
	return flows[len(flows)-limit:]
}

func plaintextCaptureColumnIDs() []string {
	ids := []string{}
	for _, col := range cfg.Columns {
		if col.Feature == plaintextCaptureFeature && col.Field != "" {
			ids = append(ids, col.ID)
		}
	}
	return ids
}

func appendPlaintextColumns(cols []string) []string {
	if capture != Packet || !plaintextCaptureEnabled() {
		return cols
	}
	if len(selectedColumns) > 0 {
		return cols
	}
	for _, id := range append([]string{"RecordType"}, plaintextCaptureColumnIDs()...) {
		if !slices.Contains(cols, id) {
			cols = append(cols, id)
		}
	}
	return cols
}

func restoreTableSelection() {
	if tableView == nil || selectedFlowIndex < 0 {
		return
	}
	for i, f := range tableData.flows {
		if idx, ok := f["Index"].(int); ok && idx == selectedFlowIndex {
			suppressTableSelection = true
			tableView.Select(i+1, 0)
			suppressTableSelection = false
			return
		}
	}
}

func appendToTimeSortedBuffer(buf *[]config.GenericMap, m config.GenericMap, maxLen int) {
	*buf = append(*buf, m)
	sort.Slice(*buf, func(i, j int) bool {
		if capture == Flow {
			return toFloat64((*buf)[i], "TimeFlowEndMs") < toFloat64((*buf)[j], "TimeFlowEndMs")
		}
		if capture == Packet {
			return comparePacketFlows((*buf)[i], (*buf)[j])
		}
		return toFloat64((*buf)[i], "Time") < toFloat64((*buf)[j], "Time")
	})
	if len(*buf) > maxLen {
		*buf = (*buf)[len(*buf)-maxLen:]
	}
}

func clearPacketCaptureBuffers() {
	lastWirePackets = nil
	lastPlaintextFlows = nil
}

func packetCaptureFlowSnapshot() []config.GenericMap {
	if capture == Packet && plaintextCaptureEnabled() {
		merged := make([]config.GenericMap, 0, len(lastWirePackets)+len(lastPlaintextFlows))
		merged = append(merged, lastWirePackets...)
		merged = append(merged, lastPlaintextFlows...)
		sortPacketFlows(merged)
		return merged
	}
	snapshot := make([]config.GenericMap, len(lastFlows))
	copy(snapshot, lastFlows)
	return snapshot
}

func AppendFlow(genericMap config.GenericMap) {
	mutex.Lock()
	if paused {
		mutex.Unlock()
		return
	}

	// add new flow to the array
	genericMap["Index"] = flowIndex
	flowIndex++

	if capture == Packet && plaintextCaptureEnabled() {
		switch {
		case isPlaintextRecord(genericMap):
			appendToTimeSortedBuffer(&lastPlaintextFlows, genericMap, defaultPlaintextKeepCount)
		case isWirePacketRecord(genericMap):
			appendToTimeSortedBuffer(&lastWirePackets, genericMap, defaultWirePacketKeepCount)
		default:
			appendToTimeSortedBuffer(&lastFlows, genericMap, keepCount)
		}
		mutex.Unlock()
		return
	}

	lastFlows = append(lastFlows, genericMap)

	// sort flows according to time
	sort.Slice(lastFlows, func(i, j int) bool {
		if capture == Flow {
			return toFloat64(lastFlows[i], "TimeFlowEndMs") < toFloat64(lastFlows[j], "TimeFlowEndMs")
		}
		if capture == Packet {
			return comparePacketFlows(lastFlows[i], lastFlows[j])
		}
		return toFloat64(lastFlows[i], "Time") < toFloat64(lastFlows[j], "Time")
	})

	// limit flows kept in memory
	if len(lastFlows) > keepCount {
		lastFlows = lastFlows[len(lastFlows)-keepCount:]
	}

	mutex.Unlock()
}

func updateDisplayEnrichmentTexts() {
	displayTextView.SetText(getDisplayText())
	enrichmentTextView.SetText(getEnrichmentText())
}

func getCols() []string {
	cols := []string{}
	if len(selectedColumns) > 0 {
		cols = selectedColumns
	} else if display.getCurrentItem().name == rawDisplay {
		cols = append(cols,
			rawDisplay,
		)
	} else {
		// main field, always show the end time
		cols = append(cols,
			"EndTime",
		)

		// enrichment fields
		if enrichment.getCurrentItem().name != noOptions {
			cols = append(cols, enrichment.getCurrentItem().ids...)
		} else {
			// TODO: add a new flag in the config to identify these as default non enriched fields
			cols = append(cols,
				"SrcAddr",
				"SrcPort",
				"DstAddr",
				"DstPort",
			)
		}

		// append interfaces and their directions between enrichment and features
		// this is useful for pkt drop, udns etc
		cols = append(cols,
			"Interfaces",
			"IfDirections",
		)

		// standard / feature fields
		if display.getCurrentItem().name != standardDisplay {
			for _, col := range cfg.Columns {
				if col.Field != "" && slices.Contains(display.getCurrentItem().ids, col.Feature) {
					cols = append(cols, col.ID)
				}
			}
		} else {
			// TODO: add a new flag in the config to identify these as default feature fields
			cols = append(cols,
				"FlowDirection",
				"Proto",
				"Dscp",
				"Bytes",
				"Packets",
			)
		}
	}
	return appendPlaintextColumns(cols)
}

func getFlows() []config.GenericMap {
	mutex.Lock()
	lfCopy := packetCaptureFlowSnapshot()
	mutex.Unlock()

	// Flow capture only: keep a selected row visible briefly after it leaves the buffer.
	// Packet capture evicts purely by time so the table tracks live traffic.
	if capture != Packet {
		indexes := []int{}
		for _, lf := range lfCopy {
			indexes = append(indexes, lf["Index"].(int))
		}
		missingFlows := []config.GenericMap{}
		for _, flow := range tableData.flows {
			if !slices.Contains(indexes, flow["Index"].(int)) {
				missingFlows = append(missingFlows, flow)
			}
		}
		lfCopy = append(missingFlows, lfCopy...)
	}

	// apply regexes to filter flows
	flows := []config.GenericMap{}
	if len(regexes) > 0 {
		// regexes may change during the render so we make a copy first
		rCopy := make([]string, len(regexes))
		copy(rCopy, regexes)

		for _, flow := range lfCopy {
			match := true
			for i := range rCopy {
				ok, _ := regexp.MatchString(rCopy[i], fmt.Sprintf("%v", flow))
				match = match && ok
				if !match {
					break
				}
			}
			if match {
				flows = append(flows, flow)
			}
		}
	} else {
		flows = lfCopy
	}

	flows = trimFlowsToShowCount(flows, showCount)
	return flows
}

func getTableRows() []string {
	arr := []string{}
	if len(tableData.cols) == 0 || len(tableData.flows) == 0 {
		return arr
	}

	for i := range len(tableData.flows) + 1 {
		str := ""
		for j := range tableData.cols {
			str += tableData.GetCell(i, j).Text
		}
		arr = append(arr, str)
	}

	return arr
}

func updateTableAndSuggestions() {
	// update tableData
	tableData.cols = getCols()
	tableData.flows = getFlows()

	// update suggestions
	suggestions = []string{}
	for _, flow := range tableData.flows {
		for k, v := range flow {
			if !slices.Contains(suggestions, k) {
				suggestions = append(suggestions, k)
			}

			valueStr := fmt.Sprintf("%v", v)
			if !slices.Contains(suggestions, valueStr) {
				suggestions = append(suggestions, valueStr)
			}
		}
	}
	restoreTableSelection()
}

func selectData(data []byte, kind string) {
	selectedData = data
	selectedPayloadKind = kind
	pause(true)
}

func resetSelection() {
	selectedData = []byte{}
	selectedPayloadKind = ""
	selectedFlowIndex = -1
	pause(false)
}

func (d *TableData) GetCell(row, col int) *tview.TableCell {
	if len(d.cols) == 0 {
		return tview.NewTableCell("Initializing...")
	} else if row < 0 {
		return tview.NewTableCell("invalid row")
	} else if col < 0 || col >= len(d.cols) {
		return tview.NewTableCell("invalid col")
	}

	id := d.cols[col]
	width := toColWidth(id)
	color := tcell.ColorWhite
	bgColor := tcell.ColorBlack
	if row == 0 {
		color = tcell.ColorWhite
		bgColor = tcell.ColorBlue
	} else if col == 0 {
		color = tcell.ColorYellow
		bgColor = tcell.ColorBlack
	} else if id == "EndTime" {
		color = tcell.ColorYellow
		bgColor = tcell.ColorWhite
	}
	if row == 0 {
		return tview.NewTableCell(toColName(id, toColWidth(id))).
			SetTextColor(color).
			SetBackgroundColor(bgColor).
			SetAlign(tview.AlignLeft).
			SetMaxWidth(width)
	}
	index := row - 1
	if index < len(d.flows) {
		cellColor := color
		if isPlaintextRecord(d.flows[index]) {
			cellColor = tcell.ColorLightGreen
		}
		return tview.NewTableCell(toColValue(d.flows[index], id, toColWidth(id))).
			SetTextColor(cellColor).
			SetBackgroundColor(bgColor).
			SetAlign(tview.AlignLeft).
			SetMaxWidth(width)
	}

	// index out of bounds due to concurrent update
	return tview.NewTableCell("")
}

func (d *TableData) GetRowCount() int {
	return len(d.flows) + 1
}

func (d *TableData) GetColumnCount() int {
	return len(d.cols)
}
