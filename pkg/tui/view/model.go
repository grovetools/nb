// Package view is a tabbed meta-panel wrapping nb's browser. Single
// tab today, designed to grow into more (concept browser, inbox
// filter, etc.) without another refactor.
package view

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/grovetools/core/tui/components/pager"

	"github.com/grovetools/nb/pkg/tui/browser"
)

// Model is the nb meta-panel.
type Model struct {
	meta pager.Meta[browser.Model]
}

// New constructs a Model wrapping a fresh browser. Zero-config pager:
// nb's inner browser already renders its own left padding, so we let
// it own layout and just stack the tab bar on top via the pager's
// default View() path.
func New(cfg browser.Config) Model {
	return Model{meta: pager.Wrap(browser.New(cfg), pager.WrapConfig{
		Name: "Browser",
		Config: pager.Config{
			OuterPadding: [4]int{0, 0, 0, 0},
			FooterHeight: 1, // help line pinned from the browser's FooterView
			// One page, so the tab bar renders a single "① Browser" chip that
			// names what the pane already obviously is, and its spacer row below
			// it — two rows of the browser's height spent on nothing. The pager's
			// tab-jump and cycle keys are inert with one page either way.
			HideTabBar: true,
		},
		// The browser prefixes its layout with a leading "\n" to leave a gap
		// above its title when it runs standalone. Hosted with HideTabBar the
		// body starts at the top of the pane, so there is nothing to leave a
		// gap from. Stripping it is free rather than clipping a row, because
		// the browser drops that row from its own height accounting whenever
		// Config.Hosted is set.
		TrimLeadingNewline: true,
		// The help text is one long unwrapped line and FooterHeight reserves
		// exactly one row; a footer that wrapped to two would take the extra
		// row out of the body, which in a half-width treemux panel is where
		// it hurts.
		TruncateFooter: true,
	})}
}

func (m Model) Init() tea.Cmd { return m.meta.Init() }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.meta, cmd = m.meta.Step(msg)
	return m, cmd
}

func (m Model) View() string { return m.meta.View() }

func (m Model) Close() error { return m.meta.Close() }

// IsTextEntryActive delegates to the browser so the terminal host can
// suspend navigation bindings during text input.
func (m Model) IsTextEntryActive() bool { return m.meta.IsTextEntryActive() }

// TestState returns a snapshot of internal state for the debug API.
func (m Model) TestState() map[string]interface{} {
	return map[string]interface{}{
		"mode":       "browser",
		"note_count": m.meta.Inner().NoteCount(),
	}
}
