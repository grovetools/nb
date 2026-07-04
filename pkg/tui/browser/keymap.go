package browser

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/grovetools/core/config"
	"github.com/grovetools/core/tui/keymap"
	"github.com/grovetools/core/tui/theme"
)

// KeyMap defines the keybindings for the browser TUI.
// It embeds keymap.Base for standard navigation, actions, search, selection, and fold bindings.
// Only TUI-specific bindings that don't exist in Base are defined here.
type KeyMap struct {
	keymap.Base
	// Focus operations (TUI-specific)
	FocusEcosystem  key.Binding
	ClearFocus      key.Binding
	FocusSelected   key.Binding
	FocusRecent     key.Binding
	FocusArchive    key.Binding
	JumpToArtifacts key.Binding
	// Search operations (TUI-specific)
	ReEnterSearch key.Binding
	// Filter operations (TUI-specific)
	FilterByTag      key.Binding
	ToggleGitChanges key.Binding
	Sort             key.Binding
	CycleGrouping    key.Binding
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
	PromoteToJob     key.Binding
	Rename           key.Binding
	PriorityUp       key.Binding
	PriorityDown     key.Binding
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
	Refresh     key.Binding
	Sync        key.Binding
	AutoArchive key.Binding
}

func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Help, k.Quit}
}

// Sections returns all keybinding sections for the browser TUI.
// Only includes sections that the browser actually implements.
func (k KeyMap) Sections() []keymap.Section {
	return []keymap.Section{
		k.Base.NavigationSection(),
		// Actions (Base): confirm/back/edit/delete(dd)/yank(yy)/rename/refresh/copy-path.
		// These are all handled in update.go but were previously invisible in help.
		k.Base.ActionsSection(),
		k.Base.SelectionSection(),
		// Search plus the TUI-specific "i" re-enter-search binding.
		k.Base.SearchSection().With(k.ReEnterSearch),
		// Scoped View section: nb only implements switch-view (tab) and preview (v).
		keymap.ViewSection(k.SwitchView, k.TogglePreview),
		k.Base.FoldSection(),
		// Common sections use standard constants (icons auto-resolved)
		keymap.NewSection(keymap.SectionFocus,
			k.FocusEcosystem, k.ClearFocus,
			k.FocusSelected, k.FocusRecent, k.FocusArchive,
			k.JumpToArtifacts,
		),
		keymap.NewSection(keymap.SectionFilter,
			k.FilterByTag, k.ToggleGitChanges, k.Sort, k.CycleGrouping,
		),
		keymap.NewSection(keymap.SectionToggle,
			k.ToggleArchives, k.ToggleArtifacts, k.ToggleGlobal,
			k.ToggleHold, k.ToggleColumns,
		),
		// TUI-specific sections use explicit icons
		keymap.NewSectionWithIcon("Notes", theme.IconNote,
			k.CreateNote, k.CreateNoteInbox, k.CreateNoteGlobal,
			k.CreatePlan, k.PromoteToJob, k.Rename,
			k.PriorityUp, k.PriorityDown,
		),
		keymap.NewSectionWithIcon("Clipboard", theme.IconArchive,
			k.Cut, k.Copy, k.Paste, k.Archive, k.CopyPath,
		),
		keymap.NewSection(keymap.SectionGit,
			k.GitStageToggle, k.GitStageAll, k.GitUnstageAll, k.GitCommit,
		),
		keymap.NewSectionWithIcon("Misc", theme.IconGear,
			k.Refresh, k.Sync, k.AutoArchive,
		),
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
		FocusSelected: key.NewBinding(
			key.WithKeys("."),
			key.WithHelp(".", "focus selected"),
		),
		FocusRecent: key.NewBinding(
			key.WithKeys("f"),
			key.WithHelp("f", "focus recent"),
		),
		FocusArchive: key.NewBinding(
			key.WithKeys(","),
			key.WithHelp(",", "archive view (.archive/.closed)"),
		),
		JumpToArtifacts: key.NewBinding(
			key.WithKeys(";"),
			key.WithHelp(";", "jump to job artifacts"),
		),
		// Search operations
		ReEnterSearch: key.NewBinding(
			key.WithKeys("i"),
			key.WithHelp("i", "re-enter search (vim insert)"),
		),
		// Filter operations
		FilterByTag: key.NewBinding(
			key.WithKeys("&"),
			key.WithHelp("&", "filter by tag"),
		),
		ToggleGitChanges: key.NewBinding(
			key.WithKeys("<", ">"),
			// Help label uses "/" so it is treated as an alternate-key list and
			// not audited as a single-key label (keys are "<" and ">").
			key.WithHelp("</>", "git changes"),
		),
		Sort: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "toggle sort order"),
		),
		// NOTE: The briefing requested default key "g", but nb's browser already
		// binds the "gg" go-to-top sequence; a lone "g" is always consumed as the
		// prefix of that sequence and can never trigger a bare-key action. Per the
		// "prefer real code over the plan, and note the deviation" rule we bind
		// CycleGrouping to "o" (unused) instead. Users can remap via config.
		CycleGrouping: key.NewBinding(
			key.WithKeys("o"),
			key.WithHelp("o", "cycle group-by (none/date/status/tag/priority)"),
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
			key.WithKeys("a"),
			key.WithHelp("a", "inbox note (quick capture)"),
		),
		CreateNoteGlobal: key.NewBinding(
			key.WithKeys("I"),
			key.WithHelp("I", "global note"),
		),
		CreatePlan: key.NewBinding(
			key.WithKeys("P"),
			key.WithHelp("P", "promote note to plan"),
		),
		PromoteToJob: key.NewBinding(
			key.WithKeys("J"),
			key.WithHelp("J", "promote note to job"),
		),
		Rename: key.NewBinding(
			key.WithKeys("R"),
			key.WithHelp("R", "rename note"),
		),
		PriorityUp: key.NewBinding(
			key.WithKeys("{"),
			key.WithHelp("{", "bump priority more critical"),
		),
		PriorityDown: key.NewBinding(
			key.WithKeys("}"),
			key.WithHelp("}", "bump priority less critical"),
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
		Refresh: key.NewBinding(
			key.WithKeys("ctrl+r"),
			key.WithHelp("ctrl+r", "refresh"),
		),
		Sync: key.NewBinding(
			key.WithKeys("S"),
			key.WithHelp("S", "sync with remotes"),
		),
		// NOTE: The briefing suggested "Shift+S" for auto-archive, but "S" (and
		// thus Shift+S) is already bound to Sync. We bind AutoArchive to "Z"
		// (an unused key) instead. It is a MANUAL action only — never run on
		// startup. Users can remap via config.
		AutoArchive: key.NewBinding(
			key.WithKeys("Z"),
			key.WithHelp("Z", "auto-archive notes older than 30 days"),
		),
	}

	// The nb browser is not a tabbed pager: it does not implement the Base
	// tab/focus navigation bindings. Disable them so help stays truthful and
	// AuditCoverage does not flag them as hidden-but-enabled. The scoped View
	// section above exposes only what the browser actually handles
	// (switch-view + preview).
	km.NextTab.SetEnabled(false)
	km.PrevTab.SetEnabled(false)
	km.FocusNext.SetEnabled(false)
	km.FocusPrev.SetEnabled(false)
	km.Tab1.SetEnabled(false)
	km.Tab2.SetEnabled(false)
	km.Tab3.SetEnabled(false)
	km.Tab4.SetEnabled(false)
	km.Tab5.SetEnabled(false)
	km.Tab6.SetEnabled(false)
	km.Tab7.SetEnabled(false)
	km.Tab8.SetEnabled(false)
	km.Tab9.SetEnabled(false)

	// Apply TUI-specific overrides from config
	keymap.ApplyTUIOverrides(cfg, "nb", "browser", &km)

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
