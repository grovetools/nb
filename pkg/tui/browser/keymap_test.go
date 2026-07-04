package browser

import (
	"testing"

	"github.com/grovetools/core/tui/keymap"
)

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
