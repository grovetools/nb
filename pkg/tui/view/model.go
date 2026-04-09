// Package view is the tabbed meta-panel wrapper for the notebook TUI.
// It currently hosts nb's existing browser as its sole tab (tab 1)
// and exists primarily to establish the pager-based infrastructure —
// future expansion (concept browser, inbox filter, daemon status,
// syncthing health, memory bridge) becomes a one-line page append
// instead of a follow-on refactor.
//
// The meta-panel speaks the standard core/tui/components/pager
// contract: tab bar rendering, numeric jump keys 1-9, NextTab/PrevTab
// cycling via "]" / "[", and cross-tab auto-switch via
// embed.SwitchTabMsg. Even with a single tab the bar is rendered for
// visual consistency with cx, memory, and (eventually) flow.
package view

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/grovetools/core/tui/components/pager"
	"github.com/grovetools/core/tui/embed"
	"github.com/grovetools/nb/pkg/tui/browser"
)

// Model is the nb view meta-panel. It embeds a pager.Model with one
// page (the nb browser) and satisfies tea.Model so hosts can mount
// it directly in place of the flat browser.Model they used before.
type Model struct {
	pager pager.Model
}

// New constructs a view.Model around a freshly-built browser. The
// browser config is taken verbatim so callers don't need to know
// about the pager wrapper at construction time.
func New(cfg browser.Config) Model {
	b := browser.New(cfg)
	page := &browserPage{inner: b}
	p := pager.New([]pager.Page{page}, pager.DefaultKeyMap())
	return Model{pager: p}
}

// Init forwards to the pager, which forwards to its active page.
func (m Model) Init() tea.Cmd { return m.pager.Init() }

// Update routes messages through the pager. The pager intercepts
// WindowSizeMsg (subtracting its own tab bar height), tab jumps
// and cycling, and SwitchTabMsg; everything else is forwarded to
// the active page. The wrapped browser still sees Focus/Blur
// messages via the pager forwarding them to the active page.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.pager, cmd = m.pager.Update(msg)
	return m, cmd
}

// View renders the tab bar directly above the active page with
// minimal chrome. The wrapped nb browser already emits a leading
// newline and a PaddingLeft(2) around its whole content, so we
// can't blindly reuse pager.View() + Padding(1, 2) here — that
// double-pads horizontally and produces 2 stacked blank rows
// between the tab bar and the browser header (bar row + pager's
// own blank + browser's leading \n).
//
// Instead we render the tab bar with a matching PaddingLeft(2) so
// it aligns with the browser content below, prepend a single top
// margin row, and let the browser's leading "\n" provide the
// one blank row separator between bar and content.
func (m Model) View() string {
	bar := lipgloss.NewStyle().PaddingLeft(2).Render(m.pager.RenderTabBar())
	body := ""
	if active := m.pager.Active(); active != nil {
		body = active.View()
	}
	return "\n" + bar + body
}

// Close is a no-op today. nb's browser model doesn't own any
// resources that need explicit teardown (unlike flow's status which
// has a daemon SSE stream); the method exists so hosts can call it
// uniformly across all ecosystem meta-panels.
func (m Model) Close() error { return nil }

// browserPage adapts nb's browser.Model to the pager.Page interface.
// The browser is a value-receiver bubbletea model, so Update returns
// a fresh copy each time; the adapter stores the latest on itself
// and rewraps on return.
type browserPage struct {
	inner  browser.Model
	width  int
	height int
}

func (p *browserPage) Name() string { return "Browser" }

func (p *browserPage) Init() tea.Cmd { return p.inner.Init() }

func (p *browserPage) Update(msg tea.Msg) (pager.Page, tea.Cmd) {
	updated, cmd := p.inner.Update(msg)
	if bm, ok := updated.(browser.Model); ok {
		p.inner = bm
	}
	return p, cmd
}

func (p *browserPage) View() string { return p.inner.View() }

// Focus delivers embed.FocusMsg to the inner model. The browser's
// Update loop handles focus via the embed contract, so we don't
// need a distinct Focus method on browser.Model itself.
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

// SetSize is called by the pager when the host sends WindowSizeMsg.
// It forwards the dimensions to the browser as a WindowSizeMsg so the
// inner model's existing resize path handles viewport recalculation.
func (p *browserPage) SetSize(w, h int) {
	p.width = w
	p.height = h
	updated, _ := p.inner.Update(tea.WindowSizeMsg{Width: w, Height: h})
	if bm, ok := updated.(browser.Model); ok {
		p.inner = bm
	}
}
