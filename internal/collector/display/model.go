package display

import (
	"github.com/gdamore/tcell/v2"
	collectorflow "github.com/netobserv/network-observability-cli/internal/collector/flow"
	"github.com/rivo/tview"
)

// Model contains display data shared by the flow and metrics views.
// Capture orchestration remains in collector; this package owns the data
// structures consumed by the terminal renderer.
type Model struct {
	TableData  *TableData
	FlowBuffer *collectorflow.Buffer
	Graphs     []Graph
	Colors     []tcell.Color
}

func NewModel() *Model {
	return &Model{
		TableData:  NewTableData(),
		FlowBuffer: collectorflow.NewBuffer(),
		Graphs:     []Graph{},
		Colors: []tcell.Color{
			tcell.ColorWhite, tcell.ColorPeru, tcell.ColorRebeccaPurple, tcell.ColorChocolate,
			tcell.ColorGold, tcell.ColorGreen, tcell.ColorBlue, tcell.ColorMaroon,
			tcell.ColorAquaMarine, tcell.ColorDarkSeaGreen, tcell.ColorOrange,
			tcell.ColorYellowGreen, tcell.ColorTeal, tcell.ColorPurple, tcell.ColorMintCream,
			tcell.ColorMistyRose, tcell.ColorSeaGreen, tcell.ColorSalmon, tcell.ColorMidnightBlue,
			tcell.ColorDeepSkyBlue, tcell.ColorYellow, tcell.ColorMediumSeaGreen,
			tcell.ColorBlanchedAlmond, tcell.ColorDarkKhaki, tcell.ColorHoneydew,
		},
	}
}

// Views contains the persistent widgets shared by the flow and metrics views.
// It deliberately contains no capture logic; callers decide how the widgets
// are populated and refreshed.
type Views struct {
	App                    *tview.Application
	Pages                  *tview.Pages
	MainView               *tview.Flex
	PlayPauseButton        *tview.Button
	DurationText, SizeText *tview.TextView
	CountTextView          *tview.TextView
	GraphsContainer        *tview.Flex
	PanelsTextView         *tview.TextView
	LegendView             *tview.Flex
	TableView              *tview.Table
	FiltersView            *tview.Flex
	DisplayTextView        *tview.TextView
	EnrichmentTextView     *tview.TextView
	InputField             *tview.InputField
}

func NewViews() *Views {
	return &Views{
		App:                tview.NewApplication(),
		DurationText:       tview.NewTextView(),
		SizeText:           tview.NewTextView(),
		CountTextView:      tview.NewTextView(),
		GraphsContainer:    tview.NewFlex().SetDirection(tview.FlexRow),
		PanelsTextView:     tview.NewTextView(),
		DisplayTextView:    tview.NewTextView(),
		EnrichmentTextView: tview.NewTextView(),
	}
}
