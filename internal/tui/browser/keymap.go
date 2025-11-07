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
	ToggleView      key.Binding
	Search          key.Binding
	Sort            key.Binding
	JumpToWorkspace key.Binding
	PageUp          key.Binding
	PageDown        key.Binding
	GoToTop         key.Binding
	GoToBottom      key.Binding
	FoldPrefix      key.Binding // z key for fold commands
	ToggleArchives  key.Binding
}

func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Help, k.Quit}
}

func (k KeyMap) FullHelp() [][]key.Binding {
	return k.Base.FullHelp()
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
		key.WithKeys("a"),
		key.WithHelp("a", "toggle archives"),
	),
}
