package browser

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/grovetools/core/config"
	"github.com/grovetools/core/tui/keymap"
)

// KeyMap defines the keybindings for the browser TUI.
// It embeds keymap.Base for standard navigation, actions, search, selection, and fold bindings.
// Only TUI-specific bindings that don't exist in Base are defined here.
type KeyMap struct {
	keymap.Base
	// Focus operations (TUI-specific)
	FocusEcosystem key.Binding
	ClearFocus     key.Binding
	FocusParent    key.Binding
	FocusSelected  key.Binding
	FocusRecent    key.Binding
	JumpToWorkspace key.Binding
	// Filter operations (TUI-specific)
	FilterByTag      key.Binding
	ToggleGitChanges key.Binding
	Sort             key.Binding
	// Toggle operations (TUI-specific)
	ToggleArchives  key.Binding
	ToggleArtifacts key.Binding
	ToggleGlobal    key.Binding
	ToggleHold      key.Binding
	ToggleColumns   key.Binding
	// Note operations (TUI-specific)
	CreateNote       key.Binding
	CreateNoteInbox  key.Binding
	CreateNoteGlobal key.Binding
	CreatePlan       key.Binding
	Rename           key.Binding
	// Clipboard operations (TUI-specific)
	Cut     key.Binding
	Copy    key.Binding
	Paste   key.Binding
	Archive key.Binding
	// Git operations (TUI-specific)
	GitCommit      key.Binding
	GitStageToggle key.Binding
	GitStageAll    key.Binding
	GitUnstageAll  key.Binding
	// Misc operations (TUI-specific)
	Preview key.Binding
	Refresh key.Binding
	Sync    key.Binding
}

func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Help, k.Quit}
}

// Sections returns all keybinding sections for the browser TUI.
// Only includes sections that the browser actually implements.
func (k KeyMap) Sections() []keymap.Section {
	return []keymap.Section{
		k.Base.NavigationSection(),
		k.Base.SelectionSection(),
		k.Base.SearchSection(),
		k.Base.FoldSection(),
		{
			Name: "Focus",
			Bindings: []key.Binding{
				k.FocusEcosystem, k.ClearFocus, k.FocusParent,
				k.FocusSelected, k.FocusRecent, k.JumpToWorkspace,
			},
		},
		{
			Name: "Filter",
			Bindings: []key.Binding{k.FilterByTag, k.ToggleGitChanges, k.Sort},
		},
		{
			Name: "Toggle",
			Bindings: []key.Binding{
				k.ToggleArchives, k.ToggleArtifacts, k.ToggleGlobal,
				k.ToggleHold, k.ToggleColumns,
			},
		},
		{
			Name: "Notes",
			Bindings: []key.Binding{
				k.CreateNote, k.CreateNoteInbox, k.CreateNoteGlobal,
				k.CreatePlan, k.Rename,
			},
		},
		{
			Name: "Clipboard",
			Bindings: []key.Binding{k.Cut, k.Copy, k.Paste, k.Archive, k.CopyPath},
		},
		{
			Name: "Git",
			Bindings: []key.Binding{k.GitStageToggle, k.GitStageAll, k.GitUnstageAll, k.GitCommit},
		},
		{
			Name: "Misc",
			Bindings: []key.Binding{k.Preview, k.Refresh, k.Sync},
		},
		k.Base.SystemSection(),
	}
}

// NewKeyMap creates a new KeyMap with user configuration applied.
// Base bindings (navigation, actions, search, selection, fold) come from keymap.Load().
// Only TUI-specific bindings are defined here.
func NewKeyMap(cfg *config.Config) KeyMap {
	km := KeyMap{
		Base: keymap.Load(cfg, "nb.browser"),
		// Focus operations
		FocusEcosystem: key.NewBinding(
			key.WithKeys("@"),
			key.WithHelp("@", "focus ecosystem"),
		),
		ClearFocus: key.NewBinding(
			key.WithKeys("ctrl+g"),
			key.WithHelp("ctrl+g", "clear focus"),
		),
		FocusParent: key.NewBinding(
			key.WithKeys(""),
			key.WithHelp("", "focus parent (disabled)"),
		),
		FocusSelected: key.NewBinding(
			key.WithKeys("."),
			key.WithHelp(".", "focus selected"),
		),
		FocusRecent: key.NewBinding(
			key.WithKeys("f"),
			key.WithHelp("f", "focus recent"),
		),
		JumpToWorkspace: key.NewBinding(
			key.WithKeys("1", "2", "3", "4", "5", "6", "7", "8", "9"),
			key.WithHelp("1-9", "jump to workspace"),
		),
		// Filter operations
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
		// Toggle operations
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
		ToggleColumns: key.NewBinding(
			key.WithKeys("V"),
			key.WithHelp("V", "toggle columns"),
		),
		// Note operations
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
		CreatePlan: key.NewBinding(
			key.WithKeys("P"),
			key.WithHelp("P", "promote note to plan"),
		),
		Rename: key.NewBinding(
			key.WithKeys("R"),
			key.WithHelp("R", "rename note"),
		),
		// Clipboard operations
		Cut: key.NewBinding(
			key.WithKeys("x"),
			key.WithHelp("x", "cut selected"),
		),
		Copy: key.NewBinding(
			key.WithKeys("y", "c"),
			key.WithHelp("y/c", "copy selected"),
		),
		Paste: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "paste from clipboard"),
		),
		Archive: key.NewBinding(
			key.WithKeys("X"),
			key.WithHelp("X", "archive selected"),
		),
		// Git operations
		GitCommit: key.NewBinding(
			key.WithKeys("C"),
			key.WithHelp("C", "git commit"),
		),
		GitStageToggle: key.NewBinding(
			key.WithKeys("-"),
			key.WithHelp("-", "toggle git stage"),
		),
		GitStageAll: key.NewBinding(
			key.WithKeys("="),
			key.WithHelp("=", "stage all"),
		),
		GitUnstageAll: key.NewBinding(
			key.WithKeys("+"),
			key.WithHelp("+", "unstage all"),
		),
		// Misc operations
		Preview: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "toggle preview focus"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("ctrl+r"),
			key.WithHelp("ctrl+r", "refresh"),
		),
		Sync: key.NewBinding(
			key.WithKeys("S"),
			key.WithHelp("S", "sync with remotes"),
		),
	}

	// Apply TUI-specific overrides from config (uses reflection to map all bindings)
	if cfg != nil && cfg.TUI != nil && cfg.TUI.Keybindings != nil {
		if nbOverrides, ok := cfg.TUI.Keybindings.Overrides["nb"]; ok {
			if overrides, ok := nbOverrides["browser"]; ok {
				keymap.ApplyOverrides(&km, overrides)
			}
		}
	}

	return km
}

// KeymapInfo returns the keymap metadata for the nb browser TUI.
// Used by the grove keys registry generator to aggregate all TUI keybindings.
func KeymapInfo() keymap.TUIInfo {
	km := NewKeyMap(nil)
	return keymap.MakeTUIInfo(
		"nb-browser",
		"nb",
		"Notebook browser and note manager",
		km,
	)
}
