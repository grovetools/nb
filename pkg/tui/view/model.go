// Package view is a tabbed meta-panel wrapping nb's browser. Single
// tab today, designed to grow into more (concept browser, inbox
// filter, etc.) without another refactor.
package view

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/grovetools/core/tui/components/pager"
	"github.com/grovetools/core/tui/embed"
	"github.com/grovetools/nb/pkg/tui/browser"
)

// Model is the nb meta-panel.
type Model struct {
	pager pager.Model
}

// New constructs a Model wrapping a fresh browser.
func New(cfg browser.Config) Model {
	b := browser.New(cfg)
	page := &browserPage{inner: b}
	return Model{pager: pager.New([]pager.Page{page}, pager.DefaultKeyMap())}
}

func (m Model) Init() tea.Cmd { return m.pager.Init() }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.pager, cmd = m.pager.Update(msg)
	return m, cmd
}

// View renders bar + browser body. The browser already adds its own
// PaddingLeft(2) and a leading "\n", so we left-pad the bar to align
// and rely on the browser's leading newline as the bar/body separator
// instead of double-padding.
func (m Model) View() string {
	bar := lipgloss.NewStyle().PaddingLeft(2).Render(m.pager.RenderTabBar())
	body := ""
	if active := m.pager.Active(); active != nil {
		body = active.View()
	}
	return "\n" + bar + body
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
func (p *browserPage) View() string  { return p.inner.View() }

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
