package views

import (
	"testing"
	"time"

	"github.com/grovetools/nb/pkg/models"
	"github.com/grovetools/nb/pkg/tree"
)

func revealTestItem(group, filename, priority string) *tree.Item {
	return noteToItem(&models.Note{
		Path:      "/tmp/ws/nb/" + group + "/" + filename,
		Title:     filename,
		Group:     group,
		Workspace: "demo",
		Priority:  priority,
		CreatedAt: time.Now(),
	})
}

// cursorGroupKey names where the cursor parked, for readable failures.
func cursorGroupKey(m *Model) string {
	n := m.GetCurrentNode()
	if n == nil || n.Item == nil {
		return "<none>"
	}
	if key := n.GroupKey(); key != "" {
		return key
	}
	return n.Item.Path
}

// expandedGroups lists the group rows currently rendered, which is how the
// "everything else folded" property is asserted: a collapsed sibling still
// renders its own row, but none of its notes.
func visibleGroupNames(m *Model) []string {
	var out []string
	for _, n := range m.displayNodes {
		if n.IsGroup() {
			out = append(out, n.Item.Name)
		}
	}
	return out
}

func TestRevealGroupFoldsEverythingElse(t *testing.T) {
	m, _ := newTreeTestModel(t)
	m.allItems = []*tree.Item{
		revealTestItem("inbox", "a.md", "p0"),
		revealTestItem("inbox", "b.md", ""),
		revealTestItem("issues", "c.md", ""),
	}
	// Start from a fully-open tree so the assertion is about RevealGroup, not
	// about the initial state.
	m.rebuildTree()

	if !m.RevealGroup("demo", "issues", "") {
		t.Fatalf("RevealGroup reported miss; cursor at %s", cursorGroupKey(m))
	}
	if got := cursorGroupKey(m); got != "demo:issues" {
		t.Fatalf("cursor = %s, want demo:issues", got)
	}
	// Both group rows stay visible (the workspace is open), but neither group's
	// notes do — the target included: the jump lands on a folded row.
	if len(visibleNotePaths(m)) != 0 {
		t.Fatalf("expected every group folded, visible notes = %v", visibleNotePaths(m))
	}
	if len(visibleGroupNames(m)) < 2 {
		t.Fatalf("expected sibling group rows to remain visible, got %v", visibleGroupNames(m))
	}
}

func TestRevealGroupDescendsIntoPriorityBucket(t *testing.T) {
	m, _ := newTreeTestModel(t)
	m.allItems = []*tree.Item{
		revealTestItem("inbox", "urgent.md", "p0"),
		revealTestItem("inbox", "routine.md", ""),
	}
	m.rebuildTree()

	if !m.RevealGroup("demo", "inbox", "p0") {
		t.Fatalf("RevealGroup(p0) reported miss; cursor at %s", cursorGroupKey(m))
	}
	if m.groupBy != "priority" {
		t.Fatalf("groupBy = %q, want priority", m.groupBy)
	}
	node := m.GetCurrentNode()
	if node == nil || node.Item == nil || node.Item.Name != "P0" {
		t.Fatalf("cursor = %s, want the P0 bucket", cursorGroupKey(m))
	}
}

func TestRevealGroupUnknownGroupLandsOnWorkspace(t *testing.T) {
	m, _ := newTreeTestModel(t)
	m.allItems = []*tree.Item{revealTestItem("inbox", "a.md", "")}
	m.rebuildTree()

	if m.RevealGroup("demo", "does-not-exist", "") {
		t.Fatal("RevealGroup reported a hit for a group that does not exist")
	}
	node := m.GetCurrentNode()
	if node == nil || !node.IsWorkspace() {
		t.Fatalf("cursor = %s, want the workspace row", cursorGroupKey(m))
	}
}
