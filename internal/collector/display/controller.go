package display

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Controller owns the tview application lifecycle. Domain packages provide
// the page builder and input handler, keeping display infrastructure independent
// from collector state.
type Controller struct {
	Application *tview.Application
	build       func() tview.Primitive
	handleKey   func(*tcell.EventKey) *tcell.EventKey
}

func NewController(build func() tview.Primitive, handleKey func(*tcell.EventKey) *tcell.EventKey) *Controller {
	return &Controller{
		Application: tview.NewApplication(),
		build:       build,
		handleKey:   handleKey,
	}
}

func (c *Controller) Run() error {
	c.Application.SetInputCapture(c.handleKey).
		SetRoot(c.build(), true).
		EnableMouse(true)
	return c.Application.Run()
}

func (c *Controller) Draw() { c.Application.Draw() }

func (c *Controller) SetRoot(root tview.Primitive) {
	c.Application.SetRoot(root, true)
}

type PagesOptions struct {
	Main      func() tview.Primitive
	Modal     func() tview.Primitive
	ShowModal bool
}

func Pages(options PagesOptions) *tview.Pages {
	pages := tview.NewPages().AddPage("main", options.Main(), true, true)
	if options.ShowModal && options.Modal != nil {
		pages.AddPage("modal", options.Modal(), true, true)
	}
	return pages
}
