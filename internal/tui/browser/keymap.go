package browser

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/mattsolo1/grove-core/tui/keymap"
)

// KeyMap defines the keybindings for the browser TUI
type KeyMap struct {
	keymap.Base
	FocusEcosystem  key.Binding
	ClearFocus      key.Binding
	FocusParent     key.Binding
	FocusSelected   key.Binding
	ToggleView      key.Binding
	Search          key.Binding
	Sort            key.Binding
	JumpToWorkspace key.Binding
	PageUp          key.Binding
	PageDown        key.Binding
	GoToTop         key.Binding
	GoToBottom      key.Binding
	FoldPrefix      key.Binding // z key for fold commands
	ToggleArchives key.Binding
	ToggleGlobal   key.Binding
	ToggleSelect   key.Binding
	SelectAll      key.Binding
	SelectNone     key.Binding
	Archive        key.Binding
	Preview        key.Binding
	Grep           key.Binding
}

func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Help, k.Quit}
}

func (k KeyMap) FullHelp() [][]key.Binding {
	baseHelp := k.Base.FullHelp()
	return append(baseHelp, []key.Binding{
		k.FocusEcosystem,
		k.ClearFocus,
		k.FocusParent,
		k.FocusSelected,
		k.ToggleView,
		k.Search,
		k.Grep,
		k.Sort,
		k.JumpToWorkspace,
	}, []key.Binding{
		k.PageUp,
		k.PageDown,
		k.GoToTop,
		k.GoToBottom,
		k.FoldPrefix,
		k.ToggleArchives,
		k.ToggleGlobal,
	}, []key.Binding{
		k.ToggleSelect,
		k.SelectAll,
		k.SelectNone,
		k.Archive,
		k.Preview,
	})
}

var keys = KeyMap{
	Base: keymap.NewBase(),
	FocusEcosystem: key.NewBinding(
		key.WithKeys("@"),
		key.WithHelp("@", "focus ecosystem"),
	),
	ClearFocus: key.NewBinding(
		key.WithKeys("ctrl+g"),
		key.WithHelp("ctrl+g", "clear focus"),
	),
	FocusParent: key.NewBinding(
		key.WithKeys("-"),
		key.WithHelp("-", "focus parent"),
	),
	FocusSelected: key.NewBinding(
		key.WithKeys("."),
		key.WithHelp(".", "focus selected"),
	),
	ToggleView: key.NewBinding(
		key.WithKeys("t"),
		key.WithHelp("t", "toggle view"),
	),
	Search: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "search"),
	),
	Sort: key.NewBinding(
		key.WithKeys("s"),
		key.WithHelp("s", "sort"),
	),
	JumpToWorkspace: key.NewBinding(
		key.WithKeys("1", "2", "3", "4", "5", "6", "7", "8", "9"),
		key.WithHelp("1-9", "jump to workspace"),
	),
	PageUp: key.NewBinding(
		key.WithKeys("ctrl+u"),
		key.WithHelp("ctrl+u", "page up"),
	),
	PageDown: key.NewBinding(
		key.WithKeys("ctrl+d"),
		key.WithHelp("ctrl+d", "page down"),
	),
	GoToTop: key.NewBinding(
		key.WithKeys("g"),
		key.WithHelp("gg", "go to top"),
	),
	GoToBottom: key.NewBinding(
		key.WithKeys("G"),
		key.WithHelp("G", "go to bottom"),
	),
	FoldPrefix: key.NewBinding(
		key.WithKeys("z"),
		key.WithHelp("z", "fold commands (za/zo/zc/zM/zR)"),
	),
	ToggleArchives: key.NewBinding(
		key.WithKeys("A"),
		key.WithHelp("A", "toggle archives"),
	),
	ToggleGlobal: key.NewBinding(
		key.WithKeys("~"),
		key.WithHelp("~", "toggle global"),
	),
	ToggleSelect: key.NewBinding(
		key.WithKeys(" "),
		key.WithHelp("space", "toggle select"),
	),
	SelectAll: key.NewBinding(
		key.WithKeys("a"),
		key.WithHelp("a", "select all (visible)"),
	),
	SelectNone: key.NewBinding(
		key.WithKeys("n"),
		key.WithHelp("n", "deselect all"),
	),
	Archive: key.NewBinding(
		key.WithKeys("x"),
		key.WithHelp("x", "archive selected"),
	),
	Preview: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "preview note"),
	),
	Grep: key.NewBinding(
		key.WithKeys("*"),
		key.WithHelp("*", "grep content"),
	),
}
