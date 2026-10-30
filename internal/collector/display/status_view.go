package display

import "github.com/rivo/tview"

type InfoRowOptions struct {
	CaptureText  string
	LogLevelText string
	ShowLogLevel bool
	DurationText string
	CaptureSize  string
	Paused       bool
	Debug        bool
	FPS          int
	OnPause      func()
	OnFPSChange  func(int)
}

func InfoRow(options *InfoRowOptions) (*tview.Flex, *tview.Button, *tview.TextView, *tview.TextView) {
	playPause := tview.NewButton(PlayPauseText(options.Paused)).SetSelectedFunc(options.OnPause)
	duration := tview.NewTextView().SetText(options.DurationText).SetTextAlign(tview.AlignCenter)
	size := tview.NewTextView().SetText(options.CaptureSize).SetTextAlign(tview.AlignCenter)
	row := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(tview.NewTextView().SetText(options.CaptureText), 0, 1, false).
		AddItem(playPause, 10, 0, false)
	if options.ShowLogLevel {
		row.AddItem(tview.NewTextView().SetText(options.LogLevelText), 0, 1, false)
	}
	row.AddItem(duration, 0, 1, false).
		AddItem(size, 0, 1, false)
	if options.Debug {
		fpsText := tview.NewTextView().SetText(FPSText(options.FPS)).SetTextAlign(tview.AlignCenter)
		row.AddItem(fpsText, 0, 1, false).
			AddItem(tview.NewButton("-").SetSelectedFunc(func() {
				if options.FPS > 1 {
					options.FPS--
					options.OnFPSChange(options.FPS)
					fpsText.SetText(FPSText(options.FPS))
				}
			}), 5, 0, false).
			AddItem(tview.NewButton("+").SetSelectedFunc(func() {
				options.FPS++
				options.OnFPSChange(options.FPS)
				fpsText.SetText(FPSText(options.FPS))
			}), 5, 0, false)
	}
	row.AddItem(tview.NewTextView(), 16, 0, false)
	return row, playPause, duration, size
}

type CountRowOptions struct {
	Text      func(int) string
	Value     int
	Minimum   int
	Step      int
	UseSpacer bool
	OnChange  func(int)
}

func CountRow(options CountRowOptions) *tview.Flex {
	text := tview.NewTextView().SetText(options.Text(options.Value))
	row := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(text, 0, 1, false).
		AddItem(tview.NewButton("-").SetSelectedFunc(func() {
			if options.Value-options.Step >= options.Minimum {
				options.Value -= options.Step
				text.SetText(options.Text(options.Value))
				options.OnChange(options.Value)
			}
		}), 5, 0, false).
		AddItem(tview.NewButton("+").SetSelectedFunc(func() {
			options.Value += options.Step
			text.SetText(options.Text(options.Value))
			options.OnChange(options.Value)
		}), 5, 0, false).
		AddItem(tview.NewTextView(), 0, 2, false)
	if options.UseSpacer {
		row.AddItem(tview.NewTextView(), 16, 0, false)
	}
	return row
}
