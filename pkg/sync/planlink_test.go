package sync

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/grovetools/nb/pkg/frontmatter"
	"github.com/grovetools/nb/pkg/models"
)

// fixtureNote writes a note into a fixture notebook directory and returns the
// models.Note the syncer would have listed for it. Tests run against fixture
// notebooks only (D7) — never a real notebook root.
func fixtureNote(t *testing.T, dir, name, fmBody string) *models.Note {
	t.Helper()
	path := filepath.Join(dir, name)
	content := "---\n" + fmBody + "\n---\n\nBody.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture note: %v", err)
	}
	fm, _, err := frontmatter.Parse(content)
	if err != nil {
		t.Fatalf("parse fixture note: %v", err)
	}
	return &models.Note{Path: path, PlanRef: fm.PlanRef}
}

func TestPlanLinkIndexMatchesBranchAndWorktreeStamps(t *testing.T) {
	dir := t.TempDir()

	notes := []*models.Note{
		fixtureNote(t, dir, "by-branch.md", `id: t1
title: Ticket by branch
aliases: []
tags: []
created: 2023-01-01 10:00:00
modified: 2023-01-01 10:00:00
plan_ref: plans/hosted-git-and-prs
branch: hosted-git-and-prs`),
		fixtureNote(t, dir, "by-worktree.md", `id: t2
title: Ticket by worktree
aliases: []
tags: []
created: 2023-01-01 10:00:00
modified: 2023-01-01 10:00:00
plan_ref: plans/nav-review
worktree: nav-review-column`),
		// Not promoted: has stamps but no plan_ref, so it must not index.
		fixtureNote(t, dir, "unpromoted.md", `id: t3
title: Not promoted
aliases: []
tags: []
created: 2023-01-01 10:00:00
modified: 2023-01-01 10:00:00
branch: orphan-branch`),
	}

	idx := buildPlanLinkIndex(notes)

	tests := []struct {
		name    string
		item    *Item
		wantRef string
	}{
		{
			name:    "pr head branch matches a ticket branch stamp",
			item:    &Item{Type: "pr", HeadBranch: "hosted-git-and-prs"},
			wantRef: "plans/hosted-git-and-prs",
		},
		{
			name:    "pr head branch matches a ticket worktree stamp",
			item:    &Item{Type: "pull_request", HeadBranch: "nav-review-column"},
			wantRef: "plans/nav-review",
		},
		{
			name:    "no matching stamp leaves the link absent",
			item:    &Item{Type: "pr", HeadBranch: "some-other-branch"},
			wantRef: "",
		},
		{
			name:    "a stamp without a plan_ref does not link",
			item:    &Item{Type: "pr", HeadBranch: "orphan-branch"},
			wantRef: "",
		},
		{
			name:    "a PR with no head branch does not link",
			item:    &Item{Type: "pr", HeadBranch: ""},
			wantRef: "",
		},
		{
			name:    "issues never link, even on a matching name",
			item:    &Item{Type: "issue", HeadBranch: "hosted-git-and-prs"},
			wantRef: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := idx.planRefForItem(tt.item); got != tt.wantRef {
				t.Errorf("planRefForItem() = %q, want %q", got, tt.wantRef)
			}
		})
	}
}

// TestPlanLinkIndexRefusesAmbiguousBranches pins the no-guessing rule: when two
// plans claim one branch there is no correct answer, so there is no link.
func TestPlanLinkIndexRefusesAmbiguousBranches(t *testing.T) {
	dir := t.TempDir()

	notes := []*models.Note{
		fixtureNote(t, dir, "a.md", `id: a
title: A
aliases: []
tags: []
created: 2023-01-01 10:00:00
modified: 2023-01-01 10:00:00
plan_ref: plans/alpha
branch: shared-branch`),
		fixtureNote(t, dir, "b.md", `id: b
title: B
aliases: []
tags: []
created: 2023-01-01 10:00:00
modified: 2023-01-01 10:00:00
plan_ref: plans/beta
branch: shared-branch`),
	}

	idx := buildPlanLinkIndex(notes)

	if got := idx.planRefForItem(&Item{Type: "pr", HeadBranch: "shared-branch"}); got != "" {
		t.Errorf("planRefForItem(ambiguous) = %q, want \"\" (no guessing)", got)
	}
}

// TestPlanLinkIndexIgnoresPathDerivedBranch guards the trap that makes
// models.Note.Branch unusable here: service.GetNoteMetadata reports "main" for
// every note in the centralized layout, so indexing it would link every PR
// whose head branch is "main" to an arbitrary plan.
func TestPlanLinkIndexIgnoresPathDerivedBranch(t *testing.T) {
	dir := t.TempDir()

	note := fixtureNote(t, dir, "no-stamp.md", `id: t
title: Promoted but unstamped
aliases: []
tags: []
created: 2023-01-01 10:00:00
modified: 2023-01-01 10:00:00
plan_ref: plans/hosted-git-and-prs`)
	// What ListAllNotes would have produced for a note with no branch stamp.
	note.Branch = "main"

	idx := buildPlanLinkIndex([]*models.Note{note})

	if got := idx.planRefForItem(&Item{Type: "pr", HeadBranch: "main"}); got != "" {
		t.Errorf("planRefForItem(\"main\") = %q, want \"\" (path-derived branch must not index)", got)
	}
}

func TestApplyPlanLinkNeverOverwritesOrClears(t *testing.T) {
	dir := t.TempDir()
	notes := []*models.Note{
		fixtureNote(t, dir, "t.md", `id: t
title: Ticket
aliases: []
tags: []
created: 2023-01-01 10:00:00
modified: 2023-01-01 10:00:00
plan_ref: plans/derived
branch: feature-x`),
	}
	idx := buildPlanLinkIndex(notes)
	s := &Syncer{logger: testLogger()}

	t.Run("fills an empty plan_ref", func(t *testing.T) {
		fm := &frontmatter.Frontmatter{}
		s.applyPlanLink(fm, &Item{Type: "pr", HeadBranch: "feature-x"}, idx)
		if fm.PlanRef != "plans/derived" {
			t.Errorf("PlanRef = %q, want plans/derived", fm.PlanRef)
		}
	})

	t.Run("leaves an existing plan_ref alone", func(t *testing.T) {
		fm := &frontmatter.Frontmatter{PlanRef: "plans/human-chose-this"}
		s.applyPlanLink(fm, &Item{Type: "pr", HeadBranch: "feature-x"}, idx)
		if fm.PlanRef != "plans/human-chose-this" {
			t.Errorf("PlanRef = %q, want the existing value to survive", fm.PlanRef)
		}
	})

	t.Run("never clears a plan_ref when there is no match", func(t *testing.T) {
		fm := &frontmatter.Frontmatter{PlanRef: "plans/existing"}
		s.applyPlanLink(fm, &Item{Type: "pr", HeadBranch: "unknown-branch"}, idx)
		if fm.PlanRef != "plans/existing" {
			t.Errorf("PlanRef = %q, want plans/existing", fm.PlanRef)
		}
	})

	t.Run("tolerates a nil index", func(t *testing.T) {
		fm := &frontmatter.Frontmatter{}
		s.applyPlanLink(fm, &Item{Type: "pr", HeadBranch: "feature-x"}, nil)
		if fm.PlanRef != "" {
			t.Errorf("PlanRef = %q, want empty when PRs are not mirrored", fm.PlanRef)
		}
	})
}
