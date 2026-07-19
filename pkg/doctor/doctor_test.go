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
	completed := filepath.Join(root, "completed")

	// Live and archived plan directories.
	require.NoError(t, os.MkdirAll(filepath.Join(plansDir, "liveplan"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(plansDir, "orphanplan"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(plansDir, "movedplan"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(plansDir, "deadplan"), 0o755))
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

	// Relink target: an unlinked note in inbox that a live plan job points at by
	// a still-valid absolute path.
	orphanTarget := writeNote(t, inbox, "orphan-target", "", "")

	// Relink target reachable only after a lifecycle move: the job's note_ref is
	// a stale in_progress path, but the note now lives in completed/.
	writeNote(t, completed, "moved-target", "", "")

	// Jobs. liveplan/01-live.md is claimed by the live note (id-shaped note_ref,
	// doesn't resolve, but the note already claims it). orphanplan/01-orphan.md's
	// note_ref is a legacy absolute path to the still-unlinked inbox note.
	writeJob(t, filepath.Join(plansDir, "liveplan"), "01-live.md", "live")
	writeJob(t, filepath.Join(plansDir, "orphanplan"), "01-orphan.md", orphanTarget)
	writeJob(t, filepath.Join(plansDir, "movedplan"), "01-moved.md",
		filepath.Join(inProgress, "moved-target.md"))
	writeJob(t, filepath.Join(plansDir, "deadplan"), "01-dead.md",
		"/absolute/path/to/workspace/in_progress/note.md")

	return root
}

// jobEntry returns the report's entry for plan/jobFile.
func jobEntry(t *testing.T, r *Report, plan, jobFile string) *JobEntry {
	t.Helper()
	for i := range r.UnclaimedJobs {
		if r.UnclaimedJobs[i].Plan == plan && r.UnclaimedJobs[i].JobFile == jobFile {
			return &r.UnclaimedJobs[i]
		}
	}
	t.Fatalf("no unclaimed job entry for %s/%s", plan, jobFile)
	return nil
}

// readNoteRef returns the note_ref recorded in a job file's frontmatter.
func readNoteRef(t *testing.T, path string) string {
	t.Helper()
	jfm := parseJobFrontmatter(path)
	require.NotNil(t, jfm, "frontmatter in %s", path)
	return jfm.NoteRef
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

	// Forward direction: three jobs are unclaimed; liveplan/01-live.md is claimed
	// by the live note and must NOT appear. Each gets a planned repair.
	require.Len(t, r.UnclaimedJobs, 3)
	assert.Equal(t, RepairRelink, jobEntry(t, r, "orphanplan", "01-orphan.md").Repair)
	assert.Equal(t, RepairRelink, jobEntry(t, r, "movedplan", "01-moved.md").Repair)
	assert.Equal(t, RepairClear, jobEntry(t, r, "deadplan", "01-dead.md").Repair)

	// The stale in_progress path resolved to the note's new home in completed/.
	assert.Equal(t, filepath.Join(root, "completed", "moved-target.md"),
		jobEntry(t, r, "movedplan", "01-moved.md").ResolvedNote)

	// Read-only runs advertise the repair they would make.
	assert.Contains(t, jobEntry(t, r, "movedplan", "01-moved.md").ProposedFix(), "would relink")
	assert.Contains(t, jobEntry(t, r, "deadplan", "01-dead.md").ProposedFix(), "would clear")

	// Summary: 7 note problems (2 archived, 1 gone, 3 malformed, 1 unlinked) + 3 jobs.
	assert.Equal(t, 2, r.Summary["live"])
	assert.Equal(t, 2, r.Summary["archived"])
	assert.Equal(t, 1, r.Summary["gone"])
	assert.Equal(t, 3, r.Summary["malformed_legacy"])
	assert.Equal(t, 1, r.Summary["unlinked"])
	assert.Equal(t, 3, r.Summary["unclaimed_jobs"])
	assert.Equal(t, 2, r.Summary["jobs_relinked"])
	assert.Equal(t, 1, r.Summary["jobs_cleared"])
	assert.Equal(t, 0, r.Summary["jobs_ambiguous"])
	assert.Equal(t, 10, r.Summary["problems"])
	assert.Equal(t, 0, r.Summary["actions_taken"])
	assert.Equal(t, 10, r.ProblemsRemaining())

	// Read-only: nothing moved or rewritten, on either side of the link.
	assert.FileExists(t, filepath.Join(root, "in_progress", "unlinked.md"))
	assert.FileExists(t, filepath.Join(root, "in_progress", "archived.md"))
	assert.Equal(t, "/absolute/path/to/workspace/in_progress/note.md",
		readNoteRef(t, filepath.Join(root, "plans", "deadplan", "01-dead.md")))
}

func TestRun_FixHealsEverything(t *testing.T) {
	silenceEvents(t)
	root := buildFixture(t)

	r, err := Run(root, "testws", true)
	require.NoError(t, err)
	assert.Equal(t, 0, r.ProblemsRemaining())
	assert.Equal(t, 10, r.Summary["actions_taken"])

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

	// UNCLAIMED job with a live path ref: note link backfilled, and the job's
	// note_ref rewritten from the path to the note's stable id.
	require.Len(t, r.UnclaimedJobs, 3)
	assert.NotEmpty(t, jobEntry(t, r, "orphanplan", "01-orphan.md").ActionTaken)
	pr, pj = readLink(t, filepath.Join(inbox, "orphan-target.md"))
	assert.Equal(t, "plans/orphanplan", pr)
	assert.Equal(t, "01-orphan.md", pj)
	assert.Equal(t, "orphan-target",
		readNoteRef(t, filepath.Join(root, "plans", "orphanplan", "01-orphan.md")))

	// UNCLAIMED job with a STALE path ref: resolved by basename to the note's
	// new home in completed/, then relinked the same way.
	pr, pj = readLink(t, filepath.Join(completed, "moved-target.md"))
	assert.Equal(t, "plans/movedplan", pr)
	assert.Equal(t, "01-moved.md", pj)
	assert.Equal(t, "moved-target",
		readNoteRef(t, filepath.Join(root, "plans", "movedplan", "01-moved.md")))

	// UNCLAIMED job whose ref resolves to nothing: the dead hint is cleared, and
	// no note is invented for it.
	assert.Equal(t, "", readNoteRef(t, filepath.Join(root, "plans", "deadplan", "01-dead.md")))
	assert.Equal(t, "cleared dead note_ref", jobEntry(t, r, "deadplan", "01-dead.md").ActionTaken)
}

// TestSetJobNoteRef_PreservesOtherFields guards the surgical line rewrite: the
// job's other frontmatter (including fields doctor does not model) and its body
// must survive a note_ref rewrite untouched.
func TestSetJobNoteRef_PreservesOtherFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "01-job.md")
	original := "---\nid: job-1\ntitle: job\nstatus: pending\nnote_ref: /old/path.md\nrules_file: rules/01-job.md.rules\ncustom_field: [a, b]\n---\n\nprompt body\nnote_ref: not-frontmatter\n"
	require.NoError(t, os.WriteFile(path, []byte(original), 0o644))

	require.NoError(t, setJobNoteRef(path, "note-abc"))
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t,
		strings.Replace(original, "note_ref: /old/path.md", "note_ref: note-abc", 1),
		string(got))

	// Clearing writes the explicit empty form flow itself emits.
	require.NoError(t, setJobNoteRef(path, ""))
	assert.Equal(t, "", readNoteRef(t, path))
	got, err = os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(got), "note_ref: \"\"")
	assert.Contains(t, string(got), "custom_field: [a, b]")
	assert.Contains(t, string(got), "note_ref: not-frontmatter")
}

// TestRun_AmbiguousNoteRefIsNeverGuessed: when a stale path's basename matches
// more than one assignable note, doctor refuses to pick and the job stays a
// problem (keeping the exit code non-zero) even under --fix.
func TestRun_AmbiguousNoteRefIsNeverGuessed(t *testing.T) {
	silenceEvents(t)
	root := filepath.Join(t.TempDir(), "workspaces", "testws")
	plansDir := filepath.Join(root, "plans")
	require.NoError(t, os.MkdirAll(filepath.Join(plansDir, "ambigplan"), 0o755))

	// Two same-named notes in different lifecycle dirs, neither carrying an id
	// that matches the filename stem, so the id tie-break cannot narrow them.
	writeNote(t, filepath.Join(root, "inbox"), "dupe", "", "")
	writeNote(t, filepath.Join(root, "completed"), "dupe", "", "")
	rewriteID(t, filepath.Join(root, "inbox", "dupe.md"), "id-a")
	rewriteID(t, filepath.Join(root, "completed", "dupe.md"), "id-b")

	writeJob(t, filepath.Join(plansDir, "ambigplan"), "01-ambig.md",
		filepath.Join(root, "in_progress", "dupe.md"))

	r, err := Run(root, "testws", true)
	require.NoError(t, err)

	j := jobEntry(t, r, "ambigplan", "01-ambig.md")
	assert.Equal(t, RepairAmbiguous, j.Repair)
	assert.Len(t, j.Candidates, 2)
	assert.Empty(t, j.ActionTaken, "ambiguous jobs must not be mutated")
	assert.Equal(t, 1, r.ProblemsRemaining(), "ambiguity stays a problem after --fix")
	assert.Equal(t, 1, r.Summary["jobs_ambiguous"])

	// Nothing was written: both notes stay unlinked and the ref is untouched.
	pr, pj := readLink(t, filepath.Join(root, "inbox", "dupe.md"))
	assert.Equal(t, "", pr)
	assert.Equal(t, "", pj)
	assert.Equal(t, filepath.Join(root, "in_progress", "dupe.md"),
		readNoteRef(t, filepath.Join(plansDir, "ambigplan", "01-ambig.md")))
}

// TestRun_StaleRefToNoteOwnedByAnotherJobIsCleared: the note is the source of
// truth, so a hint pointing at a note that already belongs to a different job
// is dead weight and gets cleared rather than stealing the note.
func TestRun_StaleRefToNoteOwnedByAnotherJobIsCleared(t *testing.T) {
	silenceEvents(t)
	root := filepath.Join(t.TempDir(), "workspaces", "testws")
	plansDir := filepath.Join(root, "plans")
	require.NoError(t, os.MkdirAll(filepath.Join(plansDir, "ownerplan"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(plansDir, "thiefplan"), 0o755))

	// The note already claims ownerplan/01-owned.md.
	owned := writeNote(t, filepath.Join(root, "completed"), "owned", "plans/ownerplan", "01-owned.md")
	writeJob(t, filepath.Join(plansDir, "ownerplan"), "01-owned.md", "owned")
	writeJob(t, filepath.Join(plansDir, "thiefplan"), "01-thief.md", owned)

	r, err := Run(root, "testws", true)
	require.NoError(t, err)

	// ownerplan's job is claimed, so only thiefplan's shows up — and it clears.
	require.Len(t, r.UnclaimedJobs, 1)
	j := jobEntry(t, r, "thiefplan", "01-thief.md")
	assert.Equal(t, RepairClear, j.Repair)
	assert.Equal(t, 0, r.ProblemsRemaining())
	assert.Equal(t, "", readNoteRef(t, filepath.Join(plansDir, "thiefplan", "01-thief.md")))

	// The note's existing link is untouched.
	pr, pj := readLink(t, owned)
	assert.Equal(t, "plans/ownerplan", pr)
	assert.Equal(t, "01-owned.md", pj)
}

// TestRun_TwoJobsOneNote: when two jobs point at the same note, the first (in
// deterministic plan/job order) wins it and the second's hint is cleared — no
// note is ever double-claimed.
func TestRun_TwoJobsOneNote(t *testing.T) {
	silenceEvents(t)
	root := filepath.Join(t.TempDir(), "workspaces", "testws")
	plansDir := filepath.Join(root, "plans")
	require.NoError(t, os.MkdirAll(filepath.Join(plansDir, "aplan"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(plansDir, "bplan"), 0o755))

	target := writeNote(t, filepath.Join(root, "inbox"), "shared", "", "")
	writeJob(t, filepath.Join(plansDir, "aplan"), "01-a.md", target)
	writeJob(t, filepath.Join(plansDir, "bplan"), "01-b.md", target)

	r, err := Run(root, "testws", true)
	require.NoError(t, err)
	assert.Equal(t, RepairRelink, jobEntry(t, r, "aplan", "01-a.md").Repair)
	assert.Equal(t, RepairClear, jobEntry(t, r, "bplan", "01-b.md").Repair)
	assert.Equal(t, 0, r.ProblemsRemaining())

	pr, pj := readLink(t, target)
	assert.Equal(t, "plans/aplan", pr)
	assert.Equal(t, "01-a.md", pj)
}

// TestRun_UnreadableNoteIsReportedNotCleared: a hint that resolves to a note
// whose frontmatter is corrupt (an unquoted colon in the title — common in this
// notebook) must NOT be cleared, since the note demonstrably exists; doctor
// can't rewrite what it can't parse, so it reports the note for manual repair.
func TestRun_UnreadableNoteIsReportedNotCleared(t *testing.T) {
	silenceEvents(t)
	root := filepath.Join(t.TempDir(), "workspaces", "testws")
	plansDir := filepath.Join(root, "plans")
	require.NoError(t, os.MkdirAll(filepath.Join(plansDir, "corruptplan"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "completed"), 0o755))

	// `title: treemux: ...` is not valid YAML.
	notePath := filepath.Join(root, "completed", "broken.md")
	broken := "---\nid: broken-1\ntitle: treemux: restarted TUI loses agents\naliases: []\n---\n\nbody\n"
	require.NoError(t, os.WriteFile(notePath, []byte(broken), 0o644))

	jobPath := filepath.Join(plansDir, "corruptplan", "01-broken.md")
	writeJob(t, filepath.Join(plansDir, "corruptplan"), "01-broken.md",
		filepath.Join(root, "in_progress", "broken.md"))

	r, err := Run(root, "testws", true)
	require.NoError(t, err)

	j := jobEntry(t, r, "corruptplan", "01-broken.md")
	assert.Equal(t, RepairManual, j.Repair)
	assert.Equal(t, notePath, j.ResolvedNote, "the note is found, just not parseable")
	assert.Empty(t, j.ActionTaken)
	assert.Equal(t, 1, r.ProblemsRemaining())
	assert.Equal(t, 1, r.Summary["jobs_manual"])
	assert.Equal(t, 0, r.Summary["jobs_cleared"], "a found-but-corrupt note must not be treated as dead")

	// Neither file was touched.
	got, err := os.ReadFile(notePath)
	require.NoError(t, err)
	assert.Equal(t, broken, string(got))
	assert.Equal(t, filepath.Join(root, "in_progress", "broken.md"), readNoteRef(t, jobPath))
}

// rewriteID replaces a note's frontmatter id, for fixtures that need an id
// distinct from the filename stem.
func rewriteID(t *testing.T, path, id string) {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	lines := strings.Split(string(content), "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "id:") {
			lines[i] = "id: " + id
			break
		}
	}
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644))
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

	// Forward direction specifically: a relinked job is now claimed via the
	// note's link, and a cleared job carries no hint — neither re-surfaces.
	assert.Equal(t, "orphan-target",
		readNoteRef(t, filepath.Join(root, "plans", "orphanplan", "01-orphan.md")))
	assert.Equal(t, "moved-target",
		readNoteRef(t, filepath.Join(root, "plans", "movedplan", "01-moved.md")))
	assert.Equal(t, "", readNoteRef(t, filepath.Join(root, "plans", "deadplan", "01-dead.md")))
}

// TestRun_AmbiguityIsIdempotent: repeated --fix runs over an unresolvable
// ambiguity must keep reporting it without ever mutating anything.
func TestRun_AmbiguityIsIdempotent(t *testing.T) {
	silenceEvents(t)
	root := filepath.Join(t.TempDir(), "workspaces", "testws")
	plansDir := filepath.Join(root, "plans")
	require.NoError(t, os.MkdirAll(filepath.Join(plansDir, "ambigplan"), 0o755))
	writeNote(t, filepath.Join(root, "inbox"), "dupe", "", "")
	writeNote(t, filepath.Join(root, "completed"), "dupe", "", "")
	rewriteID(t, filepath.Join(root, "inbox", "dupe.md"), "id-a")
	rewriteID(t, filepath.Join(root, "completed", "dupe.md"), "id-b")
	jobPath := filepath.Join(plansDir, "ambigplan", "01-ambig.md")
	writeJob(t, filepath.Join(plansDir, "ambigplan"), "01-ambig.md",
		filepath.Join(root, "in_progress", "dupe.md"))

	before, err := os.ReadFile(jobPath)
	require.NoError(t, err)

	for i := 0; i < 2; i++ {
		r, err := Run(root, "testws", true)
		require.NoError(t, err)
		assert.Equal(t, 1, r.ProblemsRemaining(), "run %d", i+1)
		assert.Equal(t, 0, r.Summary["actions_taken"], "run %d", i+1)
	}
	after, err := os.ReadFile(jobPath)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after))
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
