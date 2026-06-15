package browser

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"

	"github.com/grovetools/nb/pkg/service"
	"github.com/grovetools/nb/pkg/tree"
	"github.com/grovetools/nb/pkg/tui/browser/views"
)

// TestBumpSelectedPriorityOptimistic proves the optimistic-bump fix: two bumps
// in a row step p3 -> p2 -> p1. This only holds if the in-memory value updates
// between presses (without a daemon round-trip), because the second bump computes
// its new value from the first bump's result. Before the fix, the second bump
// re-read the stale daemon-indexed p3 and produced p2 again.
func TestBumpSelectedPriorityOptimistic(t *testing.T) {
	tmp := t.TempDir()
	notePath := filepath.Join(tmp, "note.md")
	content := "---\ntitle: Test\npriority: p3\n---\n\nbody\n"
	if err := os.WriteFile(notePath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	item := &tree.Item{
		Path:     notePath,
		Name:     "note.md",
		IsDir:    false,
		Type:     tree.TypeNote,
		Metadata: map[string]interface{}{"Priority": "p3", "Title": "Test"},
	}

	vm := views.New(views.KeyMap{}, map[string]bool{})
	m := &Model{
		service:     &service.Service{},
		allItems:    []*tree.Item{item},
		filterInput: textinput.New(),
		views:       vm,
		// recentNotesMode gives a flat list built straight from allItems, so the
		// test needs no workspace tree setup.
		recentNotesMode: true,
	}
	m.updateViewsState()

	// First bump: p3 -> p2 (more critical).
	m.bumpSelectedPriority(true)
	if got := views.ItemToNote(m.views.GetCurrentNode().Item).Priority; got != "p2" {
		t.Fatalf("after first bump: node priority = %q, want p2", got)
	}

	// Second bump MUST read the optimistic p2, not the stale p3: p2 -> p1.
	m.bumpSelectedPriority(true)
	if got := views.ItemToNote(m.views.GetCurrentNode().Item).Priority; got != "p1" {
		t.Fatalf("after second bump: node priority = %q, want p1 (stale-read regression)", got)
	}

	// The latest value is persisted to disk synchronously.
	persisted, err := service.ParseNote(notePath)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Priority != "p1" {
		t.Fatalf("disk priority = %q, want p1", persisted.Priority)
	}
}
