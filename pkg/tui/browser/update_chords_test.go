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
		{"toggle sort", []string{"t", "s"}, "ts", "toggle sort order"},
		{"cycle grouping", []string{"t", "o"}, "to", "cycle group-by (none/date/status/tag/priority)"},
		{"git changes filter", []string{"t", "G"}, "tG", "git changes"},
		{"rename note", []string{"c", "n"}, "cn", "rename note"},
		{"promote to plan", []string{"c", "p"}, "cp", "promote note to plan"},
		{"promote to job", []string{"c", "j"}, "cj", "promote note to job"},
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
// ResolvePending popup lists the Toggle group with rows a/j/g/h/c/p/s/o/G.
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
	wantRows := map[string]bool{
		"a": true, "j": true, "g": true, "h": true, "c": true, "p": true,
		"s": true, "o": true, "G": true,
	}
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

// TestFlatHomeAndEndSurviveTheChordSeam is the regression guard for the
// re-synthesis bug the canon-60 home/end fold makes reachable. Top now carries
// a chord ("gg") AND a flat key ("home"); the seam used to rewrite every matched
// chord to Keys()[0] unconditionally, so a flat press was silently replaced by
// the chord before dispatch. The guard only re-synthesizes when the pressed key
// is not already one of the matched binding's keys.
func TestFlatHomeAndEndSurviveTheChordSeam(t *testing.T) {
	km := NewKeyMap(nil)
	extra := chordExtra(km)

	// home matches Top through the sequence engine and must stay "home".
	host := keymap.NewWhichKeyHost(nil, km.Namespaces()...)
	msg := tea.KeyMsg{Type: tea.KeyHome}
	res, matched, _ := host.ProcessChord(msg, extra...)
	if res != keymap.ChordMatched {
		t.Fatalf("flat home: want ChordMatched, got %v", res)
	}
	if !key.Matches(msg, matched) {
		t.Fatal("flat home matched a binding that does not carry it — the guard would rewrite the key away")
	}

	// end is NOT in the chord set at all (Bottom is dispatched flat), so the
	// seam must pass it through untouched.
	host = keymap.NewWhichKeyHost(nil, km.Namespaces()...)
	endMsg := tea.KeyMsg{Type: tea.KeyEnd}
	if res, _, _ = host.ProcessChord(endMsg, extra...); res != keymap.ChordNone {
		t.Fatalf("flat end: want ChordNone, got %v", res)
	}
	if !key.Matches(endMsg, km.Bottom) {
		t.Fatal("end no longer reaches Bottom — the home/end fold is incomplete")
	}
}

// TestHomeEndConfigKeysAreGone pins the other half of canon 60 §7.1: the
// standalone Home/End bindings are deleted, not aliased. An "end"->"bottom"
// NormalizeAction alias would break the currently-clean `bottom` consistency
// check, because a separate binding on keys ["end"] matches none of
// StandardActions{"bottom", ["G"]}.
func TestHomeEndConfigKeysAreGone(t *testing.T) {
	ck := configKeys(t)
	for _, dead := range []string{"home", "end"} {
		if keys, ok := ck[dead]; ok {
			t.Errorf("ConfigKey %q is still exported with keys %v — fold it into top/bottom instead", dead, keys)
		}
	}
	if got := ck["top"]; len(got) != 2 || got[0] != "gg" || got[1] != "home" {
		t.Errorf("top keys = %v, want [gg home]", got)
	}
	if got := ck["bottom"]; len(got) != 2 || got[0] != "G" || got[1] != "end" {
		t.Errorf("bottom keys = %v, want [G end]", got)
	}
}

// TestSearchPrecedenceRule pins canon 60 §3.3 at the source: nb binds "/" , so
// n/N belong to search-next/search-prev and creation moved to the Ring-1 a/A
// pair. This is what resolves nb-browser's only intra-TUI key conflict.
func TestSearchPrecedenceRule(t *testing.T) {
	km := NewKeyMap(nil)
	if got := km.CreateNote.Keys(); len(got) != 1 || got[0] != "a" {
		t.Errorf("CreateNote keys = %v, want [a]", got)
	}
	if got := km.CreateNoteInbox.Keys(); len(got) != 1 || got[0] != "A" {
		t.Errorf("CreateNoteInbox keys = %v, want [A]", got)
	}
	if got := km.Base.SearchNext.Keys(); len(got) != 1 || got[0] != "n" {
		t.Errorf("SearchNext keys = %v, want [n]", got)
	}
	// Archive stays FLAT on X: it is both a ReservedKeys entry and a
	// StandardActions entry, so moving it to a chord would break the
	// currently-clean `archive` canonical-consistency check (§3.1 rejects `ca`).
	if got := km.Archive.Keys(); len(got) != 1 || got[0] != "X" {
		t.Errorf("Archive keys = %v, want flat [X]", got)
	}
}
