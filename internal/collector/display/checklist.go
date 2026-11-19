package display

import "github.com/rivo/tview"

type ChecklistModalOptions struct {
	Title       string
	Description string
	Items       []string
	Width       int
	Height      int
	Label       func(string) string
	Selected    func(index int) bool
	OnToggle    func(index int)
	OnReset     func()
	OnDefaults  func()
	OnSave      func()
}

func ChecklistModal(options *ChecklistModalOptions) tview.Primitive {
	content := tview.NewFlex().SetDirection(tview.FlexRow)
	content.SetBorder(true).SetTitle(options.Title)
	content.AddItem(tview.NewTextView().SetText(options.Description), 2, 0, false)

	table := tview.NewTable().SetSelectable(true, false)
	setCell := func(row int) {
		checked := "[   ]"
		if options.Selected(row) {
			checked = "[ X ]"
		}
		label := options.Items[row]
		if options.Label != nil {
			label = options.Label(label)
		}
		table.SetCell(row, 0, tview.NewTableCell(checked))
		table.SetCell(row, 1, tview.NewTableCell(label))
	}
	refresh := func() {
		for row := range options.Items {
			setCell(row)
		}
	}
	table.SetSelectedFunc(func(row, _ int) {
		if row >= 0 && row < len(options.Items) {
			options.OnToggle(row)
			setCell(row)
		}
	})
	refresh()
	content.AddItem(table, 0, 1, true)

	buttons := tview.NewFlex().SetDirection(tview.FlexColumn)
	if options.OnDefaults != nil {
		buttons.AddItem(tview.NewButton("Restore defaults").SetSelectedFunc(func() {
			options.OnDefaults()
			refresh()
		}), 0, 1, false)
	} else if options.OnReset != nil {
		buttons.AddItem(tview.NewButton("Reset").SetSelectedFunc(func() {
			options.OnReset()
			refresh()
		}), 0, 1, false)
	}
	buttons.AddItem(tview.NewTextView(), 1, 0, false)
	if options.OnDefaults != nil {
		buttons.AddItem(tview.NewButton("Reset").SetSelectedFunc(func() {
			options.OnReset()
			refresh()
		}), 0, 1, false)
		buttons.AddItem(tview.NewTextView(), 1, 0, false)
	}
	buttons.AddItem(tview.NewButton("Save").SetSelectedFunc(options.OnSave), 0, 1, false)
	content.AddItem(buttons, 1, 0, false)
	return Modal(content, options.Width, options.Height)
}
