package cmd

import (
	"os"
	"path/filepath"
	"testing"

	coremodels "github.com/grovetools/core/pkg/models"

	"github.com/grovetools/nb/pkg/models"
)

func writeNoteFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// TestFilterNotesByPlanRef_UsesIndexColumns is the regression guard for
// plan_job being a real NoteIndexEntry column: when the index already carries
// both link fields the filter must not touch the filesystem at all. The notes
// below point at paths that do not exist, so any disk re-parse would blank
// PlanJob (or drop the match) and fail the test.
func TestFilterNotesByPlanRef_UsesIndexColumns(t *testing.T) {
	notes := []*models.Note{
		{Path: "/nonexistent/a.md", PlanRef: "plans/target", PlanJob: "01-a.md"},
		{Path: "/nonexistent/b.md", PlanRef: "plans/other", PlanJob: "01-b.md"},
	}

	got := filterNotesByPlanRef(notes, "plans/target")

	if len(got) != 1 {
		t.Fatalf("got %d notes, want 1", len(got))
	}
	if got[0].Path != "/nonexistent/a.md" {
		t.Errorf("matched %q, want /nonexistent/a.md", got[0].Path)
	}
	if got[0].PlanJob != "01-a.md" {
		t.Errorf("PlanJob = %q, want 01-a.md (index column should be trusted)", got[0].PlanJob)
	}
}

// TestFilterNotesByPlanRef_BackfillsFromDisk covers the two fallbacks kept for
// stale indexes: an entry missing plan_ref entirely, and an entry from a daemon
// predating the plan_job column (plan_ref present, plan_job empty).
func TestFilterNotesByPlanRef_BackfillsFromDisk(t *testing.T) {
	dir := t.TempDir()

	stalePath := writeNoteFile(t, dir, "stale.md", `---
title: Stale
plan_ref: plans/target
plan_job: 01-stale.md
---
body
`)
	oldIndexPath := writeNoteFile(t, dir, "old-index.md", `---
title: Old Index
plan_ref: plans/target
plan_job: 02-old.md
---
body
`)
	otherPath := writeNoteFile(t, dir, "other.md", `---
title: Other
plan_ref: plans/other
---
body
`)

	notes := []*models.Note{
		// Index entry lost plan_ref: must be re-parsed to find the match.
		{Path: stalePath},
		// Pre-plan_job index entry: matches on plan_ref, needs plan_job backfilled.
		{Path: oldIndexPath, PlanRef: "plans/target"},
		{Path: otherPath},
	}

	got := filterNotesByPlanRef(notes, "plans/target")

	if len(got) != 2 {
		t.Fatalf("got %d notes, want 2", len(got))
	}
	if got[0].PlanRef != "plans/target" || got[0].PlanJob != "01-stale.md" {
		t.Errorf("stale entry = %q/%q, want plans/target/01-stale.md", got[0].PlanRef, got[0].PlanJob)
	}
	if got[1].PlanJob != "02-old.md" {
		t.Errorf("old-index entry PlanJob = %q, want 02-old.md", got[1].PlanJob)
	}
}

// TestNoteFromIndexEntry_CarriesPlanJob pins the daemon-index -> models.Note
// conversion, the seam that feeds filterNotesByPlanRef on the fast path.
func TestNoteFromIndexEntry_CarriesPlanJob(t *testing.T) {
	note := noteFromIndexEntry(&coremodels.NoteIndexEntry{
		Path:    "/notes/inbox/thing.md",
		Group:   "inbox",
		PlanRef: "plans/target",
		PlanJob: "03-thing.md",
	})

	if note.PlanRef != "plans/target" {
		t.Errorf("PlanRef = %q, want plans/target", note.PlanRef)
	}
	if note.PlanJob != "03-thing.md" {
		t.Errorf("PlanJob = %q, want 03-thing.md", note.PlanJob)
	}
}
