package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	coremodels "github.com/grovetools/core/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// silenceEvents swaps the daemon notifier for a no-op so tests don't trip the
// fire-and-forget autostart path.
func silenceEvents(t *testing.T) {
	t.Helper()
	orig := emitEvent
	emitEvent = func(coremodels.NoteEvent) {}
	t.Cleanup(func() { emitEvent = orig })
}

func writeNote(t *testing.T, dir, name, planRef, planJob string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	var fmLines string
	if planRef != "" {
		fmLines += "plan_ref: " + planRef + "\n"
	}
	if planJob != "" {
		fmLines += "plan_job: " + planJob + "\n"
	}
	content := fmt.Sprintf("---\nid: %s\ntitle: %s\naliases: []\ntags: []\ncreated: 2026-01-01T00:00:00Z\nmodified: 2026-01-01T00:00:00Z\n%s---\n\n# %s\n\nbody\n",
		name, name, fmLines, name)
	path := filepath.Join(dir, name+".md")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func writeJob(t *testing.T, planDir, name, noteRef string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(planDir, 0o755))
	content := fmt.Sprintf("---\nid: %s\ntitle: %s\nstatus: pending\nnote_ref: %s\n---\n\nprompt body\n",
		name, name, noteRef)
	require.NoError(t, os.WriteFile(filepath.Join(planDir, name), []byte(content), 0o644))
}

// buildFixture creates a workspace exercising every classification and returns
// the workspace directory. The orphan-target note (backfill subject) lives in
// inbox/ (unscanned by the note side) so its healing comes solely from the
// forward job scan.
func buildFixture(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspaces", "testws")
	plansDir := filepath.Join(root, "plans")
	inProgress := filepath.Join(root, "in_progress")
	review := filepath.Join(root, "review")
	inbox := filepath.Join(root, "inbox")

	// Live and archived plan directories.
	require.NoError(t, os.MkdirAll(filepath.Join(plansDir, "liveplan"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(plansDir, "orphanplan"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(plansDir, ".archive", "archivedplan"), 0o755))

	// in_progress notes, one per classification.
	writeNote(t, inProgress, "live", "plans/liveplan", "01-live.md")        // LIVE
	writeNote(t, inProgress, "archived", "plans/archivedplan", "")          // ARCHIVED
	writeNote(t, inProgress, "gone", "plans/goneplan", "")                  // GONE
	writeNote(t, inProgress, "malformed-live", "liveplan/02-legacy.md", "") // MALFORMED-LEGACY -> live
	writeNote(t, inProgress, "malformed-arch", "archivedplan/09-x.md", "")  // MALFORMED-LEGACY -> archived
	writeNote(t, inProgress, "malformed-gone", "goneplan/01-y.md", "")      // MALFORMED-LEGACY -> gone
	writeNote(t, inProgress, "unlinked", "", "")                            // UNLINKED

	// review notes.
	writeNote(t, review, "live-review", "plans/liveplan", "03-review.md") // LIVE
	writeNote(t, review, "arch-review", "plans/archivedplan", "")         // ARCHIVED

	// Backfill target: an unlinked note in inbox that a live plan job points at.
	orphanTarget := writeNote(t, inbox, "orphan-target", "", "")

	// Jobs. liveplan/01-live.md is claimed by the live note (id-shaped note_ref,
	// doesn't resolve, but the note already claims it). orphanplan/01-orphan.md's
	// note_ref is a legacy absolute path to the still-unlinked inbox note.
	writeJob(t, filepath.Join(plansDir, "liveplan"), "01-live.md", "live")
	writeJob(t, filepath.Join(plansDir, "orphanplan"), "01-orphan.md", orphanTarget)

	return root
}

func classOf(r *Report, base string) Classification {
	for _, n := range r.Notes {
		if filepath.Base(n.Path) == base+".md" {
			return n.Classification
		}
	}
	return ""
}

func TestRun_ReadOnlyClassifies(t *testing.T) {
	silenceEvents(t)
	root := buildFixture(t)

	r, err := Run(root, "testws", false)
	require.NoError(t, err)

	assert.Equal(t, Live, classOf(r, "live"))
	assert.Equal(t, Live, classOf(r, "live-review"))
	assert.Equal(t, Archived, classOf(r, "archived"))
	assert.Equal(t, Archived, classOf(r, "arch-review"))
	assert.Equal(t, Gone, classOf(r, "gone"))
	assert.Equal(t, MalformedLegacy, classOf(r, "malformed-live"))
	assert.Equal(t, MalformedLegacy, classOf(r, "malformed-arch"))
	assert.Equal(t, MalformedLegacy, classOf(r, "malformed-gone"))
	assert.Equal(t, Unlinked, classOf(r, "unlinked"))

	// Workspace is derived from the path.
	for _, n := range r.Notes {
		assert.Equal(t, "testws", n.Workspace)
	}

	// Forward direction: orphanplan/01-orphan.md is unclaimed; liveplan/01-live.md
	// is claimed by the live note and must NOT appear.
	require.Len(t, r.UnclaimedJobs, 1)
	assert.Equal(t, "orphanplan", r.UnclaimedJobs[0].Plan)
	assert.Equal(t, "01-orphan.md", r.UnclaimedJobs[0].JobFile)

	// Summary: 7 note problems (2 archived, 1 gone, 3 malformed, 1 unlinked) + 1 job.
	assert.Equal(t, 2, r.Summary["live"])
	assert.Equal(t, 2, r.Summary["archived"])
	assert.Equal(t, 1, r.Summary["gone"])
	assert.Equal(t, 3, r.Summary["malformed_legacy"])
	assert.Equal(t, 1, r.Summary["unlinked"])
	assert.Equal(t, 1, r.Summary["unclaimed_jobs"])
	assert.Equal(t, 8, r.Summary["problems"])
	assert.Equal(t, 0, r.Summary["actions_taken"])
	assert.Equal(t, 8, r.ProblemsRemaining())

	// Read-only: nothing moved or rewritten.
	assert.FileExists(t, filepath.Join(root, "in_progress", "unlinked.md"))
	assert.FileExists(t, filepath.Join(root, "in_progress", "archived.md"))
}

func TestRun_FixHealsEverything(t *testing.T) {
	silenceEvents(t)
	root := buildFixture(t)

	r, err := Run(root, "testws", true)
	require.NoError(t, err)
	assert.Equal(t, 0, r.ProblemsRemaining())
	assert.Equal(t, 8, r.Summary["actions_taken"])

	inProgress := filepath.Join(root, "in_progress")
	completed := filepath.Join(root, "completed")
	inbox := filepath.Join(root, "inbox")

	// ARCHIVED and GONE notes moved to completed/.
	assert.NoFileExists(t, filepath.Join(inProgress, "archived.md"))
	assert.FileExists(t, filepath.Join(completed, "archived.md"))
	assert.NoFileExists(t, filepath.Join(inProgress, "gone.md"))
	assert.FileExists(t, filepath.Join(completed, "gone.md"))
	assert.FileExists(t, filepath.Join(completed, "arch-review.md"))

	// UNLINKED moved back to inbox/.
	assert.NoFileExists(t, filepath.Join(inProgress, "unlinked.md"))
	assert.FileExists(t, filepath.Join(inbox, "unlinked.md"))

	// MALFORMED-LEGACY (live) rewritten in place to the slug form.
	assert.FileExists(t, filepath.Join(inProgress, "malformed-live.md"))
	pr, pj := readLink(t, filepath.Join(inProgress, "malformed-live.md"))
	assert.Equal(t, "plans/liveplan", pr)
	assert.Equal(t, "02-legacy.md", pj)

	// MALFORMED-LEGACY (archived) rewritten then moved to completed/.
	assert.NoFileExists(t, filepath.Join(inProgress, "malformed-arch.md"))
	pr, pj = readLink(t, filepath.Join(completed, "malformed-arch.md"))
	assert.Equal(t, "plans/archivedplan", pr)
	assert.Equal(t, "09-x.md", pj)

	// MALFORMED-LEGACY (gone) moved to completed/ (not rewritten).
	assert.FileExists(t, filepath.Join(completed, "malformed-gone.md"))

	// UNCLAIMED job backfilled the inbox note's link.
	require.Len(t, r.UnclaimedJobs, 1)
	assert.NotEmpty(t, r.UnclaimedJobs[0].ActionTaken)
	pr, pj = readLink(t, filepath.Join(inbox, "orphan-target.md"))
	assert.Equal(t, "plans/orphanplan", pr)
	assert.Equal(t, "01-orphan.md", pj)
}

func TestRun_FixIsIdempotent(t *testing.T) {
	silenceEvents(t)
	root := buildFixture(t)

	_, err := Run(root, "testws", true)
	require.NoError(t, err)

	// Second --fix run must be a clean no-op.
	r2, err := Run(root, "testws", true)
	require.NoError(t, err)
	assert.Equal(t, 0, r2.ProblemsRemaining())
	assert.Equal(t, 0, r2.Summary["problems"])
	assert.Equal(t, 0, r2.Summary["actions_taken"])
	assert.Empty(t, r2.UnclaimedJobs)

	// The only notes still scanned (in_progress/review) are healthy LIVE links.
	for _, n := range r2.Notes {
		assert.Equal(t, Live, n.Classification, "note %s", n.Path)
	}
}

// readLink parses a note file and returns its plan_ref/plan_job.
func readLink(t *testing.T, path string) (planRef, planJob string) {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	m := fmBlockRe.FindSubmatch(content)
	require.Len(t, m, 2, "frontmatter block in %s", path)
	for _, line := range strings.Split(string(m[1]), "\n") {
		if v, ok := strings.CutPrefix(line, "plan_ref:"); ok {
			planRef = strings.TrimSpace(v)
		}
		if v, ok := strings.CutPrefix(line, "plan_job:"); ok {
			planJob = strings.TrimSpace(v)
		}
	}
	return planRef, planJob
}
