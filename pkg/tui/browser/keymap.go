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
// groups ta/tj/tg/th/tc/tp plus the canon-60 additions ts/to/tG; the "c" Change
// namespace groups cn/cp/cj; the "g" Goto namespace groups gg (Base.Top), ga, gv.
// The update loop arms them through the shared WhichKeyHost sequence engine and
// View() renders the popup. Order here is the wire order ProcessChord relies on.
func (k KeyMap) Namespaces() []keymap.Namespace {
	return []keymap.Namespace{
		{Prefix: "t", Label: "Toggle", Bindings: []key.Binding{
			k.ToggleArchives, k.ToggleArtifacts, k.ToggleGlobal,
			k.ToggleHold, k.ToggleColumns, k.Base.TogglePreview,
			k.Sort, k.CycleGrouping, k.ToggleGitChanges,
		}},
		{Prefix: "c", Label: "Change", Bindings: []key.Binding{
			k.Rename, k.CreatePlan, k.PromoteToJob,
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
		// Filter keeps only the Ring-3 tag filter; sort/grouping/git-changes are
		// display toggles and moved into t… (canon 60 RULE T).
		keymap.NewSection(keymap.SectionFilter, k.FilterByTag),
		// Toggle (t…) namespace section (ta/tj/tg/th/tc/tp/ts/to/tG), rendered as
		// "Toggle (t…)" via Namespace.Section().
		ns[0].Section(),
		// Change (c…) namespace section (cn/cp/cj).
		ns[1].Section(),
		// TUI-specific sections use explicit icons. The three note MUTATORS
		// (rename / promote-to-plan / promote-to-job) live in the c… section
		// above; what remains here is creation plus the priority pair.
		keymap.NewSectionWithIcon("Notes", theme.IconNote,
			k.CreateNote, k.CreateNoteInbox, k.CreateNoteGlobal,
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
		// Toggle (t…) namespace member (canon 60 RULE T; was the `<` / `>` pair,
		// which the handler never distinguished). Chord-only, no flat aliases
		// (sign-off E4) — and it resolves the `<`/`>` cross-TUI conflicts.
		// Uppercase-in-chord keeps it distinct from `tg` (toggle global) and is
		// established house style (flow-status ships cM/cA).
		ToggleGitChanges: key.NewBinding(
			key.WithKeys("tG"),
			key.WithHelp("tG", "git changes"),
		),
		// Toggle (t…) namespace member (canon 60 RULE T; was flat `s`, which is
		// Ring-1 sync here — `S` — and sort-ish in half the fleet).
		Sort: key.NewBinding(
			key.WithKeys("ts"),
			key.WithHelp("ts", "toggle sort order"),
		),
		// Toggle (t…) namespace member (canon 60 RULE T; was flat `o`, which is
		// Ring-1 "open / primary row action" fleet-wide, §5.1). The historical
		// note here explained why grouping could not be flat `g` — moot now that
		// every cycle lives behind t….
		CycleGrouping: key.NewBinding(
			key.WithKeys("to"),
			key.WithHelp("to", "cycle group-by (none/date/status/tag/priority)"),
		),
		// Toggle (t…) namespace members. Chord-only — the legacy flat aliases
		// (A/b/~/H/V) were dropped (sign-off E4, no deprecation window). nb has no
		// flat "t" to vacate, so these migrate cleanly.
		ToggleArchives: key.NewBinding(
			key.WithKeys("ta"),
			key.WithHelp("ta", "toggle archives"),
		),
		ToggleArtifacts: key.NewBinding(
			key.WithKeys("tj"),
			key.WithHelp("tj", "toggle job artifacts"),
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
		// Note operations.
		//
		// Creation keys are Ring-1 and flat BY NAME (canon 60 §5.1): `a` creates
		// the TUI's primary noun, `A` is the secondary create form. The swap off
		// `n` is the search-precedence rule (§3.3): in a TUI that binds `/`,
		// `n`/`N` belong to search-next/search-prev, which is also what resolves
		// nb-browser's only intra-TUI key conflict (n = create_note vs the
		// promoted Base.SearchNext).
		CreateNote: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "create note at cursor"),
		),
		CreateNoteInbox: key.NewBinding(
			key.WithKeys("A"),
			key.WithHelp("A", "inbox note (quick capture)"),
		),
		CreateNoteGlobal: key.NewBinding(
			key.WithKeys("I"),
			key.WithHelp("I", "global note"),
		),
		// Change (c…) namespace members (canon 60 §3.1/§4.3). Chord-only, no flat
		// aliases (sign-off E4). These are note MUTATORS, not creations, so they
		// belong in c… while `a`/`A`/`I` stay flat. `cn` is an exact cross-TUI
		// match with flow-status's landed cn=rename; it needs the same deviation
		// entry flow-status has. Retiring flat P/J clears two 3-way conflicts.
		CreatePlan: key.NewBinding(
			key.WithKeys("cp"),
			key.WithHelp("cp", "promote note to plan"),
		),
		PromoteToJob: key.NewBinding(
			key.WithKeys("cj"),
			key.WithHelp("cj", "promote note to job"),
		),
		Rename: key.NewBinding(
			key.WithKeys("cn"),
			key.WithHelp("cn", "rename note"),
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
		key.WithHelp("enter", "open in own pane / confirm"),
	)
	km.Base.Yank.SetEnabled(false)

	// Canon 60 §7.1: fold the standalone Home/End bindings into Top/Bottom and
	// delete their ConfigKeys, rather than aliasing "end"->"bottom" in
	// NormalizeAction. The alias looks like the obvious fix and is wrong: a
	// separate `end` binding would normalize to `bottom` on keys ["end"], match
	// none of StandardActions{"bottom", ["G"]}, and flip the currently-clean
	// bottom consistency check to false. Folding keeps one canonical motion per
	// direction with both spellings on it. Home/End were dead here anyway —
	// nothing dispatched them — so this also makes the keys work for the first
	// time. flow-plan-add already ships exactly this shape.
	km.Base.Top = key.NewBinding(
		key.WithKeys("gg", "home"),
		key.WithHelp("gg/home", "top"),
	)
	km.Base.Bottom = key.NewBinding(
		key.WithKeys("G", "end"),
		key.WithHelp("G/end", "bottom"),
	)
	km.Base.Home.SetEnabled(false)
	km.Base.End.SetEnabled(false)

	// Open-mode split (hosted in treemux): enter opens the note in its own
	// pinned per-file pane; e quick-opens it in the host's singleton Editor
	// pane, replacing the buffer shown there. Override Base.Edit's generic
	// "edit" help so the help menu reflects the split (ConfigKey `edit`
	// unchanged — field is the same).
	km.Base.Edit = key.NewBinding(
		key.WithKeys("e"),
		key.WithHelp("e", "quick edit in Editor pane"),
	)

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
