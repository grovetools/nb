package sync

import (
	"os"

	"github.com/grovetools/nb/pkg/frontmatter"
	"github.com/grovetools/nb/pkg/models"
)

// planLinkIndex answers one question: "which promoted ticket owns the work on
// this git branch?" It is the symmetric half of the ticket↔PR join — the
// ticket points at its PRs via `prs:`, and a mirrored PR note points back at
// the ticket's plan via `plan_ref`.
//
// The only signal used is a ticket note's OWN `branch:`/`worktree:`
// frontmatter. Deliberately NOT used:
//   - models.Note.Branch, which falls back to the literal string "main" for
//     every note in the centralized layout (service.GetNoteMetadata) and would
//     collapse the whole notebook onto one key;
//   - the plan's job files, which do carry a branch — reading them from the
//     syncer would mean teaching sync about flow's plan format, well past
//     "trivially derivable".
//
// A branch claimed by two different plans is ambiguous and yields no link.
// Guessing is worse than an absent edge.
type planLinkIndex struct {
	byBranch  map[string]string
	ambiguous map[string]bool
}

// buildPlanLinkIndex indexes the promoted tickets among notes. Only notes that
// already carry a plan_ref are candidates, and only those are re-read from
// disk — models.Note does not carry the worktree stamp, and its Branch is
// path-derived rather than the git branch.
func buildPlanLinkIndex(notes []*models.Note) *planLinkIndex {
	idx := &planLinkIndex{
		byBranch:  make(map[string]string),
		ambiguous: make(map[string]bool),
	}

	for _, note := range notes {
		if note == nil || note.PlanRef == "" {
			continue
		}
		content, err := os.ReadFile(note.Path)
		if err != nil {
			continue
		}
		fm, _, err := frontmatter.Parse(string(content))
		if err != nil || fm == nil || fm.PlanRef == "" {
			continue
		}
		for _, key := range []string{fm.Branch, fm.Worktree} {
			if key == "" {
				continue
			}
			if existing, seen := idx.byBranch[key]; seen && existing != fm.PlanRef {
				idx.ambiguous[key] = true
				continue
			}
			idx.byBranch[key] = fm.PlanRef
		}
	}

	return idx
}

// planRefForBranch returns the plan_ref of the single promoted ticket claiming
// this branch. An empty branch, no match, or an ambiguous match all return "".
func (idx *planLinkIndex) planRefForBranch(branch string) string {
	if idx == nil || branch == "" || idx.ambiguous[branch] {
		return ""
	}
	return idx.byBranch[branch]
}

// planRefForItem resolves the link for a mirrored remote item. Issues never
// link — only a pull request has a head branch to match on.
func (idx *planLinkIndex) planRefForItem(item *Item) string {
	if item == nil || (item.Type != "pr" && item.Type != "pull_request") {
		return ""
	}
	return idx.planRefForBranch(item.HeadBranch)
}
