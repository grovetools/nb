package browser

import (
	"testing"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/grovetools/core/tui/keymap"
)

// chordExtra mirrors the `extra` binding list update.go feeds ProcessChord: the
// flat sequence chords (gg/dd/z*) plus yy (Copy), with the disabled Base.Yank
// deliberately omitted so it does not race Copy for "yy".
func chordExtra(km KeyMap) []key.Binding {
	return []key.Binding{
		km.Top, km.Delete,
		km.FoldOpen, km.FoldClose, km.FoldToggle,
		km.FoldOpenAll, km.FoldCloseAll,
		km.Copy,
	}
}

func keyMsg(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// TestChordResolution drives the shared host exactly as Model.Update does and
// asserts each two-key chord resolves to the right binding, plus that a lone
// prefix arms a resolvable namespace popup.
func TestChordResolution(t *testing.T) {
	km := NewKeyMap(nil)
	extra := chordExtra(km)

	cases := []struct {
		name     string
		keys     []string
		wantKey  string // Keys()[0] of the matched binding
		wantHelp string
	}{
		{"toggle archives", []string{"t", "a"}, "ta", "toggle archives"},
		{"goto top", []string{"g", "g"}, "gg", "top"},
		{"goto artifacts", []string{"g", "a"}, "ga", "goto job artifacts"},
		{"copy yank", []string{"y", "y"}, "yy", "copy selected"},
		{"delete", []string{"d", "d"}, "dd", "delete"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host := keymap.NewWhichKeyHost(nil, km.Namespaces()...)
			var res keymap.ChordResult
			var matched key.Binding
			for i, k := range tc.keys {
				res, matched, _ = host.ProcessChord(keyMsg(k), extra...)
				if i < len(tc.keys)-1 && res != keymap.ChordPending {
					t.Fatalf("key %q: want ChordPending mid-chord, got %v", k, res)
				}
			}
			if res != keymap.ChordMatched {
				t.Fatalf("want ChordMatched, got %v", res)
			}
			if got := matched.Keys(); len(got) == 0 || got[0] != tc.wantKey {
				t.Fatalf("want matched key %q, got %v", tc.wantKey, got)
			}
			if got := matched.Help().Desc; got != tc.wantHelp {
				t.Errorf("want help %q, got %q", tc.wantHelp, got)
			}
		})
	}
}

// TestTogglePrefixArmsPopup pins that a lone "t" arms a pending namespace whose
// ResolvePending popup lists the Toggle group with rows a/b/g/h/c/p.
func TestTogglePrefixArmsPopup(t *testing.T) {
	km := NewKeyMap(nil)
	host := keymap.NewWhichKeyHost(nil, km.Namespaces()...)

	res, _, cmd := host.ProcessChord(keyMsg("t"), chordExtra(km)...)
	if res != keymap.ChordPending {
		t.Fatalf("want ChordPending after 't', got %v", res)
	}
	if cmd == nil {
		t.Error("armed namespace prefix should return a popup show-delay tick cmd")
	}
	if !host.Armed() {
		t.Fatal("host should report Armed() after 't'")
	}

	group, _ := keymap.ResolvePending(host.Sequence.Buffer(), km.Namespaces())
	if group == nil {
		t.Fatal("ResolvePending returned no group for armed 't'")
	}
	if group.Title != "Toggle (t…)" {
		t.Errorf("want group title %q, got %q", "Toggle (t…)", group.Title)
	}
	wantRows := map[string]bool{"a": true, "b": true, "g": true, "h": true, "c": true, "p": true}
	got := map[string]bool{}
	for _, r := range group.Rows {
		got[r.Keys] = true
	}
	for k := range wantRows {
		if !got[k] {
			t.Errorf("Toggle popup missing row %q (rows: %v)", k, got)
		}
	}
}

// TestParseSearchInputTag keeps the "#tag rest" folding honest (the `&` picker
// stays this phase; a follow-up ticket owns the #-opens-picker fold).
func TestParseSearchInputTag(t *testing.T) {
	query, tag, isGrep, isTag := parseSearchInput("#tag rest")
	if isGrep {
		t.Error("'#tag rest' should not be grep mode")
	}
	if !isTag {
		t.Fatal("'#tag rest' should set tag mode")
	}
	if tag != "tag" {
		t.Errorf("want tag %q, got %q", "tag", tag)
	}
	if query != "rest" {
		t.Errorf("want within-tag query %q, got %q", "rest", query)
	}
}
