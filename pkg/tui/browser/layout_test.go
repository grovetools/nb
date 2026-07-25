package browser

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/grovetools/nb/pkg/service"
	"github.com/grovetools/nb/pkg/tree"
	"github.com/grovetools/nb/pkg/tui/browser/views"
)

// newLayoutTestModel builds a browser over n flat notes, sized by the caller.
// recentNotesMode gives a flat list built straight from allItems, so no
// workspace tree setup is needed.
func newLayoutTestModel(t *testing.T, n int) *Model {
	t.Helper()
	items := make([]*tree.Item, 0, n)
	for i := range n {
		items = append(items, &tree.Item{
			Path:     fmt.Sprintf("/tmp/n%02d.md", i),
			Name:     fmt.Sprintf("note-%02d.md", i),
			Type:     tree.TypeNote,
			Metadata: map[string]any{"Title": fmt.Sprintf("Note %02d", i), "Priority": "p3"},
		})
	}
	m := &Model{
		service:         &service.Service{},
		allItems:        items,
		filterInput:     textinput.New(),
		views:           views.New(views.KeyMap{}, map[string]bool{"TYPE": true, "STATUS": true, "MODIFIED": true}),
		recentNotesMode: true,
		columnList:      list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0),
	}
	m.updateViewsState()
	return m
}

// TestViewFillsItsHeightBudget is the guard on chromeRows(). Every row the
// model reserves in chromeRows() but does not render is a blank row the host
// pager pads out at the bottom of the pane — the defect this test exists to
// catch. With more notes than fit, the rendered height must equal the height
// the model was handed, exactly, in every layout state.
//
// The hosted cases deduct the pager's own chrome the way tui/view does
// (2 rows of tab bar + spacer, 1 reserved footer row) and strip the leading
// "\n" the way its page adapter does.
func TestViewFillsItsHeightBudget(t *testing.T) {
	const paneH, paneW = 30, 80
	const pagerChrome = 3 // tab bar + spacer + reserved footer row

	cases := []struct {
		name   string
		hosted bool
		setup  func(*Model)
	}{
		{name: "hosted/tree", hosted: true},
		{name: "hosted/table", hosted: true, setup: func(m *Model) { m.views.SetViewMode(views.TableView) }},
		{name: "hosted/tree/search", hosted: true, setup: func(m *Model) {
			m.filterInput.SetValue("n0")
			m.filterInput.Focus()
		}},
		{name: "standalone/tree"},
		{name: "standalone/table", setup: func(m *Model) { m.views.SetViewMode(views.TableView) }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newLayoutTestModel(t, 200)
			m.hosted = tc.hosted
			if tc.setup != nil {
				tc.setup(m)
			}

			budget := paneH
			if tc.hosted {
				budget -= pagerChrome
			}
			updated, _ := m.Update(tea.WindowSizeMsg{Width: paneW, Height: budget})
			out := updated.(Model).View()
			if tc.hosted {
				out = strings.TrimPrefix(out, "\n")
			}

			if got := lipgloss.Height(out); got != budget {
				t.Fatalf("rendered %d rows for a %d-row budget (%+d wasted)\n%s",
					got, budget, budget-got, out)
			}
		})
	}
}

// TestViewIsNotDoubleInset guards the horizontal reclaim: the list rows share
// the frame's single PaddingLeft(2) with the title above them. A second inset
// on the list pane pushed the rows a column right of the title and clipped a
// column off the right edge, which costs table columns in a narrow panel.
func TestViewIsNotDoubleInset(t *testing.T) {
	const paneW = 60

	m := newLayoutTestModel(t, 40)
	m.hosted = true
	m.views.SetViewMode(views.TableView)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: paneW, Height: 24})
	lines := strings.Split(strings.TrimPrefix(updated.(Model).View(), "\n"), "\n")

	// Row 0 is the title; the table's header underline is the widest row the
	// list emits, so it pins both edges of the list's inset.
	titleIndent := indentOf(lines[0])
	var underline string
	for _, l := range lines {
		if strings.Contains(l, "───") {
			underline = l
			break
		}
	}
	if underline == "" {
		t.Fatalf("no table header underline in:\n%s", strings.Join(lines, "\n"))
	}

	if got := indentOf(underline); got != titleIndent {
		t.Errorf("list inset %d columns, title inset %d — the list is double-inset", got, titleIndent)
	}
	if got := lipgloss.Width(underline); got != paneW {
		t.Errorf("list spans %d of %d available columns", got, paneW)
	}
}

func indentOf(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}
