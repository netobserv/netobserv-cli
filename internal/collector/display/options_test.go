package display

import (
	"testing"

	"github.com/rivo/tview"
	"github.com/stretchr/testify/assert"
)

func TestOptionCyclesAndWraps(t *testing.T) {
	option := Option{All: []OptionItem{{Name: "first"}, {Name: "second"}}}
	assert.Equal(t, "first", option.CurrentItem().Name)

	option.Next()
	assert.Equal(t, "second", option.CurrentItem().Name)
	option.Next()
	assert.Equal(t, "first", option.CurrentItem().Name)
	option.Prev()
	assert.Equal(t, "second", option.CurrentItem().Name)
}

func TestPagesAddsMainAndOptionalModal(t *testing.T) {
	pages := Pages(PagesOptions{
		Main:      func() tview.Primitive { return tview.NewTextView() },
		Modal:     func() tview.Primitive { return tview.NewTextView() },
		ShowModal: true,
	})
	assert.Equal(t, 2, pages.GetPageCount())
	assert.True(t, pages.HasPage("main"))
	assert.True(t, pages.HasPage("modal"))
}

func TestFilterViewRendersEmptyAndActiveStates(t *testing.T) {
	empty := FilterView(FilterViewOptions{})
	active := FilterView(FilterViewOptions{Filters: []string{"src:pod", "dst:service"}})
	assert.NotNil(t, empty)
	assert.Greater(t, active.GetItemCount(), empty.GetItemCount())
}

func TestDefaultsProvideExpectedCaptureOptions(t *testing.T) {
	display, enrichment, panels := Defaults()
	assert.Equal(t, standardDisplay, display.CurrentItem().Name)
	assert.Equal(t, "Resource", enrichment.CurrentItem().Name)
	assert.NotEmpty(t, panels.CurrentItem().IDs)
}

func TestNewViewsInitializesFlowHeaders(t *testing.T) {
	views := NewViews()

	assert.NotNil(t, views.DisplayTextView)
	assert.NotNil(t, views.EnrichmentTextView)
}
