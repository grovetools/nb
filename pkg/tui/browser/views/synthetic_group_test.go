package views

import (
	"testing"
	"time"

	workspace "github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/nb/pkg/models"
)

// Regression test for note creation under group-by buckets: the synthetic
// bucket nodes ("Today", "P0", ...) must carry the enclosing on-disk group in
// Metadata["Group"] so actions like 'n' (create note at cursor) resolve to a
// real directory instead of the bucket label.
func TestSyntheticBucketsCarryEnclosingGroup(t *testing.T) {
	m := &Model{
		groupBy:        "date",
		collapsedNodes: map[string]bool{},
	}
	ws := &workspace.WorkspaceNode{Name: "grovetools", Path: "/tmp/ws"}
	notes := []*models.Note{
		{Path: "/tmp/ws/notes/inbox/a.md", Title: "a.md", CreatedAt: time.Now(), Group: "inbox", Workspace: "grovetools"},
		{Path: "/tmp/ws/notes/inbox/b.md", Title: "b.md", CreatedAt: time.Now().AddDate(0, -2, 0), Group: "inbox", Workspace: "grovetools"},
	}

	var nodes []*DisplayNode
	m.renderSyntheticGroups(&nodes, notes, ws, "/tmp/ws/notes/inbox", "inbox", "├ ", 1, map[string]string{}, false, false)

	var buckets []*DisplayNode
	for _, n := range nodes {
		if n.IsGroup() {
			buckets = append(buckets, n)
		}
	}
	if len(buckets) < 2 {
		t.Fatalf("expected at least 2 synthetic buckets (Today + Icebox), got %d", len(buckets))
	}
	for _, b := range buckets {
		group, _ := b.Item.Metadata["Group"].(string)
		if group != "inbox" {
			t.Errorf("bucket %q: expected Metadata[Group]=%q, got %q", b.Item.Name, "inbox", group)
		}
	}
}
