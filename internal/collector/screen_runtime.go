package collector

import (
	collectordisplay "github.com/netobserv/network-observability-cli/internal/collector/display"
	"github.com/rivo/tview"
)

func getPages() *tview.Pages {
	if capture == Metric {
		ui.Pages = collectordisplay.Pages(collectordisplay.PagesOptions{
			Main: getMetricMain, Modal: getMetricsModal, ShowModal: ui.showPopup,
		})
	} else {
		ui.Pages = collectordisplay.Pages(collectordisplay.PagesOptions{
			Main: getFlowMain, Modal: getColumnsModal, ShowModal: ui.showPopup,
		})
	}

	return ui.Pages
}

func heartbeat() {
	displayRuntime().Heartbeat()
}

func backgroundHeartbeat() {
	displayRuntime().BackgroundHeartbeat()
}

func displayRuntime() collectordisplay.Runtime {
	return collectordisplay.Runtime{
		Ended:       func() bool { return captureEnded },
		IsMetrics:   func() bool { return capture == Metric },
		FPS:         func() int { return ui.framesPerSecond },
		UpdateState: updateStatusTexts,
		UpdateData: func() {
			if capture == Metric {
				updatePlots()
			} else {
				updateTableAndSuggestions()
			}
		},
		Draw: func() {
			if ui.App != nil {
				ui.App.Draw()
			}
		},
		PrintRows: func() {
			for _, row := range ui.TableData.Rows() {
				log.Println(row)
			}
		},
	}
}

func updateScreen() {
	if ui.App != nil {
		ui.showPopup = false
		ui.App.SetRoot(getPages(), true)
	}
}

const defaultFramesPerSecond = 5

// displayState is the single owner of terminal UI state.
type displayState struct {
	*collectordisplay.Model
	*collectordisplay.Views
	showCount, framesPerSecond            int
	showPopup, paused                     bool
	err                                   error
	focus                                 string
	selectedPanels                        []string
	focusedGraph                          int
	regexes, suggestions, selectedColumns []string
	extraWidth                            int
	selectedData                          []byte
	selectedPayloadKind                   string
	selectedFlowIndex                     int
	selectedTableColumn                   int
	suppressTableSelection                bool
}

var ui = displayState{
	Model: collectordisplay.NewModel(), Views: collectordisplay.NewViews(), framesPerSecond: defaultFramesPerSecond,
	selectedPanels: []string{}, focusedGraph: -1,
	extraWidth: defaultExtraWidth, selectedData: []byte{}, selectedFlowIndex: -1,
}

func getInfoRow() tview.Primitive {
	row, button, duration, size := collectordisplay.InfoRow(&collectordisplay.InfoRowOptions{
		CaptureText: getCaptureText(), LogLevelText: getLogLevelText(), ShowLogLevel: logLevel != "info",
		DurationText: collectordisplay.DurationText(currentTime(), startupTime),
		CaptureSize:  collectordisplay.CaptureSize(string(capture), totalBytes), Paused: ui.paused,
		Debug: logLevel == "debug", FPS: ui.framesPerSecond,
		OnPause:     func() { pause(!ui.paused) },
		OnFPSChange: func(fps int) { ui.framesPerSecond = fps },
	})
	ui.PlayPauseButton = button
	ui.DurationText = duration
	ui.SizeText = size
	return row
}

func getCountRow(useSpacer bool) *tview.Flex {
	step := 1
	minimum := 5
	if capture == Metric {
		step = 5
	}
	return collectordisplay.CountRow(collectordisplay.CountRowOptions{
		Text: func(int) string { return getShowCountText() }, Value: ui.showCount, Minimum: minimum, Step: step, UseSpacer: useSpacer,
		OnChange: func(value int) { ui.showCount = value; ui.CountTextView.SetText(getShowCountText()); updateScreen() },
	})
}

func getShowCountText() string {
	if capture == Metric {
		return getMetricShowCountText()
	}
	return getFlowShowCountText()
}

func updateStatusTexts() {
	ui.DurationText.SetText(collectordisplay.DurationText(currentTime(), startupTime))
	ui.SizeText.SetText(collectordisplay.CaptureSize(string(capture), totalBytes))
}

func pause(paused bool) {
	ui.paused = paused
	ui.PlayPauseButton.SetLabel(collectordisplay.PlayPauseText(ui.paused))
	updateScreen()
}
