package display

import (
	"slices"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type FilterInputOptions struct {
	Suggestions func() []string
	CanRemove   func() bool
	RemoveLast  func()
	Add         func(string)
	Refresh     func()
}

func FilterInput(options FilterInputOptions) *tview.InputField {
	input := tview.NewInputField().SetLabel("Live table filters: ").SetFieldWidth(30)
	input.SetAutocompleteFunc(func(current string) (entries []string) {
		if current == "" || options.Suggestions == nil {
			return nil
		}
		for _, suggestion := range options.Suggestions() {
			if strings.HasPrefix(strings.ToLower(suggestion), strings.ToLower(current)) {
				entries = append(entries, suggestion)
			}
		}
		if len(entries) <= 1 {
			return nil
		}
		return entries
	})
	input.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		//nolint:exhaustive
		if (event.Key() == tcell.KeyBackspace || event.Key() == tcell.KeyBackspace2) && input.GetText() == "" && options.CanRemove() {
			options.RemoveLast()
			options.Refresh()
			return nil
		}
		return event
	})
	input.SetAutocompletedFunc(func(text string, _, source int) bool {
		if source != tview.AutocompletedNavigate {
			input.SetText(text)
		}
		return source == tview.AutocompletedEnter || source == tview.AutocompletedClick
	})
	input.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			if text := input.GetText(); text != "" {
				options.Add(text)
				input.SetText("")
			}
			options.Refresh()
		}
	})
	return input
}

type FilterViewOptions struct {
	Filters  []string
	OnRemove func(index int)
}

func FilterView(options FilterViewOptions) *tview.Flex {
	view := tview.NewFlex().SetDirection(tview.FlexColumn)
	if len(options.Filters) > 0 {
		view.AddItem(tview.NewTextView().SetText("Current filters:"), 17, 0, false)
		for index, filter := range slices.Clone(options.Filters) {
			view.AddItem(tview.NewButton(filter).SetSelectedFunc(func() {
				if options.OnRemove != nil {
					options.OnRemove(index)
				}
			}), len(filter), 0, false)
			view.AddItem(tview.NewTextView(), 1, 0, false)
		}
		view.AddItem(tview.NewTextView().SetText("Press `Enter` key to add a new one and backspace to remove last one"), 0, 1, false)
	} else {
		view.AddItem(tview.NewTextView().SetText("Press `Enter` key to match multiple filters at once"), 0, 1, false)
	}
	return view
}
