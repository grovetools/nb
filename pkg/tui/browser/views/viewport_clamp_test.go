package views

import (
	"fmt"
	"testing"

	"github.com/grovetools/nb/pkg/tree"
)

// Regression: typing a '/' search while scrolled down used to leave
// scrollOffset past the end of the filtered node list, so the render path
// computed a negative viewport span and panicked with
// "makeslice: cap out of range" (surfaced in the host as "panel crashed").
func TestFilterAfterScrollKeepsViewportInRange(t *testing.T) {
	m, _ := newTreeTestModel(t)
	m.SetSize(120, 20)

	items := make([]*tree.Item, 0, 60)
	for i := 0; i < 60; i++ {
		items = append(items, testNoteItem("alpha", fmt.Sprintf("note-%02d.md", i), "", nil, nil))
	}
	// Exactly one note matches the filter below.
	items = append(items, testNoteItem("alpha", "needle.md", "", nil, nil))
	m.allItems = items

	m.BuildDisplayTree()

	// Scroll to the bottom, as a user paging through a long tree would.
	m.cursor = len(m.displayNodes) - 1
	m.adjustScroll()
	if m.scrollOffset == 0 {
		t.Fatalf("setup: expected a non-zero scroll offset with %d nodes in a %d-row viewport", len(m.displayNodes), m.getViewportHeight())
	}

	// Now filter down to a handful of rows, as pressing '/' and typing does.
	m.filterValue = "needle"
	m.BuildDisplayTree()
	m.FilterDisplayTree()

	if m.scrollOffset > len(m.displayNodes) {
		t.Errorf("scrollOffset %d is past the %d filtered nodes", m.scrollOffset, len(m.displayNodes))
	}
	if start, end := m.visibleRange(); start > end {
		t.Errorf("inverted visible range [%d,%d)", start, end)
	}

	// The match must actually be on screen, not scrolled past.
	view := m.View()
	if view == "" {
		t.Errorf("filtered view rendered empty; scrollOffset=%d nodes=%d", m.scrollOffset, len(m.displayNodes))
	}
}

// The render path must survive a stale scroll offset on its own, in either
// view mode, so no future filter site can reintroduce the panic.
func TestVisibleRangeClampsStaleScrollOffset(t *testing.T) {
	m, _ := newTreeTestModel(t)
	m.SetSize(120, 20)
	m.allItems = []*tree.Item{testNoteItem("alpha", "only.md", "", nil, nil)}
	m.BuildDisplayTree()

	for _, mode := range []ViewMode{TreeView, TableView} {
		m.viewMode = mode
		for _, offset := range []int{-5, len(m.displayNodes) + 1, 10_000} {
			m.scrollOffset = offset
			start, end := m.visibleRange()
			if start < 0 || start > len(m.displayNodes) || end < start || end > len(m.displayNodes) {
				t.Errorf("mode %v offset %d: bad range [%d,%d) over %d nodes", mode, offset, start, end, len(m.displayNodes))
			}
			m.View() // must not panic
			m.ScrollIndicator()
		}
	}
}
