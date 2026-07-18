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

// Namespaces returns the which-key chord namespaces for the browser TUI, built
// from the named KeyMap fields (so any user override applied by ApplyTUIOverrides
// is reflected — Phase-1 §4 ConfigKey-stability rule). The "t" Toggle namespace
// groups ta/tb/tg/th/tc/tp; the "g" Goto namespace groups gg (Base.Top), ga, gv.
// The update loop arms them through the shared WhichKeyHost sequence engine and
// View() renders the popup. Order here is the wire order ProcessChord relies on.
func (k KeyMap) Namespaces() []keymap.Namespace {
	return []keymap.Namespace{
		{Prefix: "t", Label: "Toggle", Bindings: []key.Binding{
			k.ToggleArchives, k.ToggleArtifacts, k.ToggleGlobal,
			k.ToggleHold, k.ToggleColumns, k.Base.TogglePreview,
		}},
		{Prefix: "g", Label: "Goto", Bindings: []key.Binding{
			k.Base.Top, k.JumpToArtifacts, k.FocusArchive,
		}},
	}
}

// Sections returns all keybinding sections for the browser TUI.
// Only includes sections that the browser actually implements.
func (k KeyMap) Sections() []keymap.Section {
	ns := k.Namespaces()
	return []keymap.Section{
		k.Base.NavigationSection(),
		// Actions (Base): confirm/back/edit/delete(dd)/yank(yy)/rename/refresh/copy-path.
		// These are all handled in update.go but were previously invisible in help.
		k.Base.ActionsSection(),
		k.Base.SelectionSection(),
		// Search plus the TUI-specific "i" re-enter-search binding.
		k.Base.SearchSection().With(k.ReEnterSearch),
		// Scoped View section: nb only implements switch-view (tab). Preview moved
		// into the Toggle (t…) namespace as `tp`.
		keymap.ViewSection(k.SwitchView),
		k.Base.FoldSection(),
		// Common sections use standard constants (icons auto-resolved)
		keymap.NewSection(keymap.SectionFocus,
			k.FocusEcosystem, k.ClearFocus,
			k.FocusSelected, k.FocusRecent,
		),
		// Goto (g…) namespace: only ga/gv are exported here — gg (Base.Top) stays
		// in the Navigation section, so exporting it again would mint a duplicate
		// `top` ConfigKey and trip ValidateRegistry's duplicate-ConfigKey error.
		keymap.NewSection("Goto (g…)", k.JumpToArtifacts, k.FocusArchive),
		keymap.NewSection(keymap.SectionFilter,
			k.FilterByTag, k.ToggleGitChanges, k.Sort, k.CycleGrouping,
		),
		// Toggle (t…) namespace section (ta/tb/tg/th/tc/tp), rendered as
		// "Toggle (t…)" via Namespace.Section().
		ns[0].Section(),
		// TUI-specific sections use explicit icons
		keymap.NewSectionWithIcon("Notes", theme.IconNote,
			k.CreateNote, k.CreateNoteInbox, k.CreateNoteGlobal,
			k.CreatePlan, k.PromoteToJob, k.Rename,
			k.PriorityUp, k.PriorityDown,
		),
		// CopyPath (ctrl+y) is already surfaced by Base.ActionsSection above;
		// it is not repeated here to keep a single `copy_path` ConfigKey.
		keymap.NewSectionWithIcon("Clipboard", theme.IconArchive,
			k.Cut, k.Copy, k.Paste, k.Archive,
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
		// Goto (g…) namespace member. Chord-only — the legacy flat "," alias was
		// dropped (sign-off E4). gg (Base.Top) shares the same prefix and fires
		// first on exact match, so ga/gv just join the pending buffer.
		FocusArchive: key.NewBinding(
			key.WithKeys("gv"),
			key.WithHelp("gv", "goto archive view"),
		),
		// Goto (g…) namespace member. Chord-only — the legacy flat ";" is gone.
		JumpToArtifacts: key.NewBinding(
			key.WithKeys("ga"),
			key.WithHelp("ga", "goto job artifacts"),
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
		// Toggle (t…) namespace members. Chord-only — the legacy flat aliases
		// (A/b/~/H/V) were dropped (sign-off E4, no deprecation window). nb has no
		// flat "t" to vacate, so these migrate cleanly.
		ToggleArchives: key.NewBinding(
			key.WithKeys("ta"),
			key.WithHelp("ta", "toggle archives"),
		),
		ToggleArtifacts: key.NewBinding(
			key.WithKeys("tb"),
			key.WithHelp("tb", "toggle artifacts"),
		),
		ToggleGlobal: key.NewBinding(
			key.WithKeys("tg"),
			key.WithHelp("tg", "toggle global"),
		),
		ToggleHold: key.NewBinding(
			key.WithKeys("th"),
			key.WithHelp("th", "toggle on-hold"),
		),
		ToggleColumns: key.NewBinding(
			key.WithKeys("tc"),
			key.WithHelp("tc", "toggle columns"),
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
		// Copy is now the vim yank chord `yy` (verb unification: yank IS copy).
		// This vacates flat `c` (the reserved change prefix) and flat `y` (the
		// shadowed Copy half). Path-copy stays on canonical ctrl+y (Base.CopyPath);
		// Base.Yank is disabled below so `yy` routes here, not to path-copy.
		Copy: key.NewBinding(
			key.WithKeys("yy"),
			key.WithHelp("yy", "copy selected"),
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
	// The TUI-specific Rename (R, "rename note") and Refresh (ctrl+r) fields
	// above shadow Base.Rename/Base.Refresh with the same keys, and update.go
	// handles rename/refresh via those top-level fields. Disable the Base copies
	// so the merged export carries a single `rename`/`refresh` ConfigKey instead
	// of a duplicate from Base.ActionsSection.
	km.Base.Rename.SetEnabled(false)
	km.Base.Refresh.SetEnabled(false)

	// Hotkey-review Phase 4 chord migration (all BEFORE ApplyTUIOverrides so user
	// config still wins):
	//   - TogglePreview: flat `v` (reserved view prefix) → `tp`, joining the
	//     Toggle namespace. ConfigKey stays `toggle_preview` (field unchanged).
	//   - Confirm: drop the `y` half (`enter,y` → `enter`). With Copy on `yy`, a
	//     flat `y` would shadow the `yy` chord; enter covers every confirm path.
	//   - Yank: disabled. Its `yy` now belongs to Copy (copy selected); path-copy
	//     stays on Base.CopyPath (ctrl+y). Disabling keeps `yy` from racing Copy.
	km.Base.TogglePreview = key.NewBinding(
		key.WithKeys("tp"),
		key.WithHelp("tp", "toggle preview"),
	)
	km.Base.Confirm = key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "confirm"),
	)
	km.Base.Yank.SetEnabled(false)

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
