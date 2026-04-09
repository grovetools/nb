// Package view is a tabbed meta-panel wrapping nb's browser. Single
// tab today, designed to grow into more (concept browser, inbox
// filter, etc.) without another refactor.
package view

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/grovetools/core/tui/components/pager"
	"github.com/grovetools/core/tui/embed"
	"github.com/grovetools/nb/pkg/tui/browser"
)

// Model is the nb meta-panel.
type Model struct {
	pager pager.Model
}

// New constructs a Model wrapping a fresh browser. Zero-config pager:
// nb's inner browser already renders its own left padding, so we let
// it own layout and just stack the tab bar on top via the pager's
// default View() path.
func New(cfg browser.Config) Model {
	b := browser.New(cfg)
	page := &browserPage{inner: b}
	return Model{pager: pager.NewWith([]pager.Page{page}, pager.DefaultKeyMap(), pager.Config{
		OuterPadding: [4]int{0, 0, 0, 0},
	})}
}

func (m Model) Init() tea.Cmd { return m.pager.Init() }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.pager, cmd = m.pager.Update(msg)
	return m, cmd
}

// View delegates entirely to the pager. The browserPage adapter
// strips the browser's leading newline so the pager's blank-row
// separator (bar → blank → body) isn't doubled up.
func (m Model) View() string {
	return m.pager.View()
}

func (m Model) Close() error { return nil }

// browserPage adapts nb's browser.Model to pager.Page.
type browserPage struct {
	inner  browser.Model
	width  int
	height int
}

func (p *browserPage) Name() string  { return "Browser" }
func (p *browserPage) Init() tea.Cmd { return p.inner.Init() }
func (p *browserPage) View() string {
	// The inner browser prefixes its own layout with a leading "\n"
	// to leave a gap above the cursor row; the pager already emits a
	// blank row between the tab bar and the body, so strip the
	// leading newline here to avoid double-spacing.
	return strings.TrimPrefix(p.inner.View(), "\n")
}

func (p *browserPage) Update(msg tea.Msg) (pager.Page, tea.Cmd) {
	updated, cmd := p.inner.Update(msg)
	if bm, ok := updated.(browser.Model); ok {
		p.inner = bm
	}
	return p, cmd
}

func (p *browserPage) Focus() tea.Cmd {
	updated, cmd := p.inner.Update(embed.FocusMsg{})
	if bm, ok := updated.(browser.Model); ok {
		p.inner = bm
	}
	return cmd
}

func (p *browserPage) Blur() {
	updated, _ := p.inner.Update(embed.BlurMsg{})
	if bm, ok := updated.(browser.Model); ok {
		p.inner = bm
	}
}

func (p *browserPage) SetSize(w, h int) {
	p.width = w
	p.height = h
	updated, _ := p.inner.Update(tea.WindowSizeMsg{Width: w, Height: h})
	if bm, ok := updated.(browser.Model); ok {
		p.inner = bm
	}
}
