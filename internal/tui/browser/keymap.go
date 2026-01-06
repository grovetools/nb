package browser

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/mattsolo1/grove-core/tui/keymap"
)

// KeyMap defines the keybindings for the browser TUI
type KeyMap struct {
	keymap.Base
	FocusEcosystem   key.Binding
	ClearFocus       key.Binding
	FocusParent      key.Binding
	FocusSelected    key.Binding
	ToggleView       key.Binding
	Search           key.Binding
	FilterByTag      key.Binding
	ToggleGitChanges key.Binding
	Sort             key.Binding
	JumpToWorkspace key.Binding
	PageUp          key.Binding
	PageDown        key.Binding
	GoToTop         key.Binding
	GoToBottom      key.Binding
	Fold            key.Binding
	Unfold          key.Binding
	FoldPrefix      key.Binding // z key for fold commands
	ToggleArchives  key.Binding
	ToggleArtifacts key.Binding
	ToggleGlobal    key.Binding
	ToggleHold      key.Binding
	ToggleSelect   key.Binding
	SelectNone     key.Binding
	Archive        key.Binding
	Preview        key.Binding
	TogglePreview  key.Binding
	Grep           key.Binding
	ToggleColumns  key.Binding
	Refresh        key.Binding
	Sync           key.Binding
	Cut              key.Binding
	Copy             key.Binding
	Yank             key.Binding
	Paste            key.Binding
	Delete           key.Binding
	CreateNote       key.Binding
	CreateNoteInbox  key.Binding
	CreateNoteGlobal key.Binding
	Rename           key.Binding
	CreatePlan       key.Binding
	FocusRecent      key.Binding
	GitCommit        key.Binding
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
		k.FilterByTag,
		k.ToggleGitChanges,
		k.Grep,
		k.Sort,
		k.FocusRecent,
		k.JumpToWorkspace,
		k.Refresh,
		k.Sync,
	}, []key.Binding{
		k.PageUp,
		k.PageDown,
		k.GoToTop,
		k.GoToBottom,
		k.Fold,
		k.Unfold,
		k.FoldPrefix,
		k.ToggleArchives,
		k.ToggleArtifacts,
		k.ToggleGlobal,
		k.ToggleHold,
	}, []key.Binding{
		k.ToggleSelect,
		k.SelectNone,
		k.Archive,
		k.Preview,
		k.TogglePreview,
		k.ToggleColumns,
	}, []key.Binding{
		k.CreatePlan,
		k.CreateNote,
		k.CreateNoteInbox,
		k.CreateNoteGlobal,
		k.Rename,
		k.Delete,
		k.Cut,
		k.Copy,
		k.Yank,
		k.Paste,
		k.GitCommit,
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
	FilterByTag: key.NewBinding(
		key.WithKeys("&"),
		key.WithHelp("&", "filter by tag"),
	),
	ToggleGitChanges: key.NewBinding(
		key.WithKeys("<", ">"),
		key.WithHelp("<,>", "git changes"),
	),
	Sort: key.NewBinding(
		key.WithKeys("s"),
		key.WithHelp("s", "toggle sort order"),
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
	Fold: key.NewBinding(
		key.WithKeys("h", "left"),
		key.WithHelp("h/←", "close fold"),
	),
	Unfold: key.NewBinding(
		key.WithKeys("l", "right"),
		key.WithHelp("l/→", "open fold"),
	),
	FoldPrefix: key.NewBinding(
		key.WithKeys("z"),
		key.WithHelp("z", "fold (zA/zO/zC/za/zo/zc/zM/zR)"),
	),
	ToggleArchives: key.NewBinding(
		key.WithKeys("A"),
		key.WithHelp("A", "toggle archives"),
	),
	ToggleArtifacts: key.NewBinding(
		key.WithKeys("b"),
		key.WithHelp("b", "toggle artifacts"),
	),
	ToggleGlobal: key.NewBinding(
		key.WithKeys("~"),
		key.WithHelp("~", "toggle global"),
	),
	ToggleHold: key.NewBinding(
		key.WithKeys("H"),
		key.WithHelp("H", "toggle on-hold"),
	),
	ToggleSelect: key.NewBinding(
		key.WithKeys(" "),
		key.WithHelp("space", "toggle select"),
	),
	SelectNone: key.NewBinding(
		key.WithKeys("N"),
		key.WithHelp("N", "deselect all"),
	),
	Archive: key.NewBinding(
		key.WithKeys("X"),
		key.WithHelp("X", "archive selected"),
	),
	Preview: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "toggle preview focus"),
	),
	TogglePreview: key.NewBinding(
		key.WithKeys("v"),
		key.WithHelp("v", "toggle preview pane"),
	),
	Grep: key.NewBinding(
		key.WithKeys("*"),
		key.WithHelp("*", "grep content"),
	),
	ToggleColumns: key.NewBinding(
		key.WithKeys("V"),
		key.WithHelp("V", "toggle columns"),
	),
	Refresh: key.NewBinding(
		key.WithKeys("ctrl+r"),
		key.WithHelp("ctrl+r", "refresh"),
	),
	Sync: key.NewBinding(
		key.WithKeys("S"),
		key.WithHelp("S", "sync with remotes"),
	),
	CreateNote: key.NewBinding(
		key.WithKeys("n"),
		key.WithHelp("n", "create note at cursor"),
	),
	CreateNoteInbox: key.NewBinding(
		key.WithKeys("i"),
		key.WithHelp("i", "inbox note (quick capture)"),
	),
	CreateNoteGlobal: key.NewBinding(
		key.WithKeys("I"),
		key.WithHelp("I", "global note"),
	),
	Rename: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "rename note"),
	),
	CreatePlan: key.NewBinding(
		key.WithKeys("P"),
		key.WithHelp("P", "promote note to plan"),
	),
	Delete: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("dd", "delete selected"),
	),
	Cut: key.NewBinding(
		key.WithKeys("x"),
		key.WithHelp("x", "cut selected"),
	),
	Copy: key.NewBinding(
		key.WithKeys("y", "c"),
		key.WithHelp("y/c", "copy selected"),
	),
	Yank: key.NewBinding(
		key.WithKeys("ctrl+y"),
		key.WithHelp("ctrl+y", "yank path"),
	),
	Paste: key.NewBinding(
		key.WithKeys("p"),
		key.WithHelp("p", "paste from clipboard"),
	),
	FocusRecent: key.NewBinding(
		key.WithKeys("f"),
		key.WithHelp("f", "focus recent"),
	),
	GitCommit: key.NewBinding(
		key.WithKeys("C"),
		key.WithHelp("C", "git commit"),
	),
}
