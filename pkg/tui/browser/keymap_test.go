package browser

import (
	"testing"

	"github.com/grovetools/core/tui/keymap"
)

// configKeys walks KeymapInfo() and returns configKey -> keys for every ENABLED
// binding, asserting no ConfigKey appears twice across Sections() (a duplicate is
// a hard registry error via ValidateRegistry).
func configKeys(t *testing.T) map[string][]string {
	t.Helper()
	info := KeymapInfo()
	out := map[string][]string{}
	for _, sec := range info.Sections {
		for _, b := range sec.Bindings {
			if !b.Enabled {
				continue
			}
			if _, dup := out[b.ConfigKey]; dup {
				t.Errorf("duplicate ConfigKey %q across Sections() (section %q, binding %q)", b.ConfigKey, sec.Name, b.Name)
			}
			out[b.ConfigKey] = b.Keys
		}
	}
	return out
}

// TestChordConfigKeyStability pins the Phase-4 chord migration: the named fields
// keep their ConfigKeys (so grove.toml overrides survive), the keys became the
// single-key chords (E4 — no legacy aliases), and each carries exactly one key.
func TestChordConfigKeyStability(t *testing.T) {
	ck := configKeys(t)
	want := map[string]string{
		"toggle_archives":   "ta",
		"toggle_artifacts":  "tb",
		"toggle_global":     "tg",
		"toggle_hold":       "th",
		"toggle_columns":    "tc",
		"toggle_preview":    "tp",
		"jump_to_artifacts": "ga",
		"focus_archive":     "gv",
		"copy":              "yy",
	}
	for cfg, key := range want {
		keys, ok := ck[cfg]
		if !ok {
			t.Errorf("ConfigKey %q missing from enabled Sections() bindings", cfg)
			continue
		}
		if len(keys) != 1 || keys[0] != key {
			t.Errorf("ConfigKey %q: want single key [%q] (E4 no-alias), got %v", cfg, key, keys)
		}
	}
}

// TestNoEnabledFlatPrefixOrShadow guards the audit outcome at the source: no
// enabled binding sits on a bare namespace prefix (t/v/c/g) or on flat `y`
// (which would shadow the yy chord), Confirm is enter-only, and Base.Yank is off.
func TestNoEnabledFlatPrefixOrShadow(t *testing.T) {
	km := NewKeyMap(nil)

	// No enabled binding may be exactly "t", "v", "c", "g", or "y".
	forbidden := map[string]bool{"t": true, "v": true, "c": true, "g": true, "y": true}
	for cfg, keys := range configKeys(t) {
		for _, k := range keys {
			if forbidden[k] {
				t.Errorf("ConfigKey %q binds forbidden flat key %q (prefix squatter / shadow)", cfg, k)
			}
		}
	}

	if got := km.Base.Confirm.Keys(); len(got) != 1 || got[0] != "enter" {
		t.Errorf("Confirm should be enter-only, got %v", got)
	}
	if km.Base.Yank.Enabled() {
		t.Error("Base.Yank should be disabled (yy belongs to Copy)")
	}
}

// TestNamespacesMembership checks the two chord namespaces and their members.
func TestNamespacesMembership(t *testing.T) {
	km := NewKeyMap(nil)
	ns := km.Namespaces()
	if len(ns) != 2 {
		t.Fatalf("want 2 namespaces (t, g), got %d", len(ns))
	}
	if ns[0].Prefix != "t" || ns[1].Prefix != "g" {
		t.Fatalf("want prefixes t,g, got %q,%q", ns[0].Prefix, ns[1].Prefix)
	}

	firstKey := func(b interface{ Keys() []string }) string {
		if k := b.Keys(); len(k) > 0 {
			return k[0]
		}
		return ""
	}
	toggleKeys := map[string]bool{}
	for _, b := range ns[0].Bindings {
		toggleKeys[firstKey(b)] = true
	}
	for _, k := range []string{"ta", "tb", "tg", "th", "tc", "tp"} {
		if !toggleKeys[k] {
			t.Errorf("Toggle namespace missing member %q", k)
		}
	}
	gotoKeys := map[string]bool{}
	for _, b := range ns[1].Bindings {
		gotoKeys[firstKey(b)] = true
	}
	for _, k := range []string{"gg", "ga", "gv"} {
		if !gotoKeys[k] {
			t.Errorf("Goto namespace missing member %q", k)
		}
	}
}

// TestKeyMapAuditCoverage asserts the browser keymap has no coverage gaps:
// every enabled key.Binding (including embedded Base) appears in exactly one
// Sections() entry, every help label matches its keys, and no enabled binding
// has empty help. This is the ecosystem-wide enforcement primitive from
// core/tui/keymap. If it fails, help and the keys registry are lying.
func TestKeyMapAuditCoverage(t *testing.T) {
	km := NewKeyMap(nil)
	gaps := keymap.AuditCoverage(km)
	if len(gaps) != 0 {
		for _, g := range gaps {
			t.Errorf("keymap coverage gap: field=%s kind=%s detail=%s", g.Field, g.Kind, g.Detail)
		}
	}
}

// TestReEnterSearchBound guards the ambiguity resolved during the hotkey
// review: nb re-enters search on "i" (not "r"). update.go routes the handler
// through this binding, so keep the key stable.
func TestReEnterSearchBound(t *testing.T) {
	km := NewKeyMap(nil)
	keys := km.ReEnterSearch.Keys()
	if len(keys) != 1 || keys[0] != "i" {
		t.Fatalf("ReEnterSearch should be bound to \"i\", got %v", keys)
	}
}
