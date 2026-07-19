package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/grovetools/core/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureMoveEvents replaces the note-event funnel with a synchronous recorder
// for the duration of a test.
func captureMoveEvents(t *testing.T) *[]models.NoteEvent {
	t.Helper()
	events := &[]models.NoteEvent{}
	orig := emitNoteEvent
	emitNoteEvent = func(e models.NoteEvent) {
		*events = append(*events, e)
	}
	t.Cleanup(func() { emitNoteEvent = orig })
	return events
}

// lifecycleDir builds an nb-shaped lifecycle directory. The "/nb/" segment is
// what makes relocateNote take its in-system simple-rename branch, and it is
// what GetNoteMetadata parses workspace/type out of.
func lifecycleDir(t *testing.T, root, group string) string {
	t.Helper()
	dir := filepath.Join(root, "nb", "repos", "test-repo", "main", group)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	return dir
}

// TestRelocateNoteEmitsMoveEventOnHookPath pins the lifecycle-hook path: flow
// shells out to `nb move <path> <group> --force` with migration off, which used
// to relocate the file through a bare os.Rename and emit nothing, leaving the
// daemon's note index stale on every lifecycle transition.
func TestRelocateNoteEmitsMoveEventOnHookPath(t *testing.T) {
	events := captureMoveEvents(t)
	root := t.TempDir()

	srcDir := lifecycleDir(t, root, "inbox")
	dstDir := lifecycleDir(t, root, "in_progress")
	src := filepath.Join(srcDir, "20250101-task.md")
	dst := filepath.Join(dstDir, "20250101-task.md")
	require.NoError(t, os.WriteFile(src, []byte("---\nid: 20250101-120000\ntitle: Task\n---\n\n# Task\n"), 0o644))

	final, err := relocateNote(src, dst, false /*applyMigrate*/, true /*force*/, false /*copy*/)
	require.NoError(t, err)
	assert.Equal(t, dst, final)
	assert.NoFileExists(t, src)
	assert.FileExists(t, dst)

	require.Len(t, *events, 1, "the hook path must emit exactly one note event")
	ev := (*events)[0]
	assert.Equal(t, models.NoteEventMoved, ev.Event)
	// Both sides of the move are required: the daemon turns Path+PrevPath into a
	// first-class rename instead of a delete+create pair.
	assert.Equal(t, dst, ev.Path)
	assert.Equal(t, src, ev.PrevPath)
	assert.Equal(t, "in_progress", ev.NoteType)
	assert.Equal(t, "inbox", ev.PrevNoteType)
	assert.Equal(t, ev.Workspace, ev.PrevWorkspace)
	assert.NotEmpty(t, ev.Workspace)
}

// TestRelocateNoteCopyEmitsCreated mirrors TransferNotes in pkg/service: --copy
// leaves the source in place, so it is a creation rather than a move.
func TestRelocateNoteCopyEmitsCreated(t *testing.T) {
	events := captureMoveEvents(t)
	root := t.TempDir()

	srcDir := lifecycleDir(t, root, "inbox")
	dstDir := lifecycleDir(t, root, "review")
	src := filepath.Join(srcDir, "20250101-task.md")
	dst := filepath.Join(dstDir, "20250101-task.md")
	require.NoError(t, os.WriteFile(src, []byte("---\nid: 20250101-120000\ntitle: Task\n---\n"), 0o644))

	_, err := relocateNote(src, dst, false, true, true /*copy*/)
	require.NoError(t, err)
	assert.FileExists(t, src, "--copy must preserve the source")

	require.Len(t, *events, 1)
	assert.Equal(t, models.NoteEventCreated, (*events)[0].Event)
	assert.Equal(t, dst, (*events)[0].Path)
}

// TestMoveToPathEmitsMoveEvent covers the explicit-destination-path variant,
// which was likewise silent.
func TestMoveToPathEmitsMoveEvent(t *testing.T) {
	events := captureMoveEvents(t)
	root := t.TempDir()

	srcDir := lifecycleDir(t, root, "inbox")
	dstDir := lifecycleDir(t, root, "completed")
	src := filepath.Join(srcDir, "20250101-task.md")
	dst := filepath.Join(dstDir, "20250101-task.md")
	require.NoError(t, os.WriteFile(src, []byte("---\nid: 20250101-120000\ntitle: Task\n---\n"), 0o644))

	require.NoError(t, moveToPath(nil, src, dst, false /*dryRun*/, false /*copy*/))

	require.Len(t, *events, 1)
	assert.Equal(t, models.NoteEventMoved, (*events)[0].Event)
	assert.Equal(t, dst, (*events)[0].Path)
	assert.Equal(t, src, (*events)[0].PrevPath)
}

// TestMoveDryRunEmitsNothing guards against emitting for work never done.
func TestMoveDryRunEmitsNothing(t *testing.T) {
	events := captureMoveEvents(t)
	root := t.TempDir()

	srcDir := lifecycleDir(t, root, "inbox")
	src := filepath.Join(srcDir, "20250101-task.md")
	require.NoError(t, os.WriteFile(src, []byte("---\nid: 20250101-120000\n---\n"), 0o644))

	require.NoError(t, moveToPath(nil, src, filepath.Join(root, "elsewhere", "x.md"), true /*dryRun*/, false))
	assert.Empty(t, *events)
}

// writeNote writes a note and stamps its mtime, so tests can build a directory
// whose newest .md file is deliberately the WRONG answer.
func writeNote(t *testing.T, path, id, title string, mtime time.Time) {
	t.Helper()
	body := "---\nid: " + id + "\ntitle: " + title + "\n---\n\n# " + title + "\n"
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	require.NoError(t, os.Chtimes(path, mtime, mtime))
}

// TestResolveMigratedPathIgnoresNewerDecoy pins the deterministic rename
// recovery. The old implementation scanned the directory for the most recently
// modified .md file, so any concurrent write in the window silently won. It was
// not even sound on its own terms: the migrator restores the original mtime, so
// the migrated file is usually NOT the newest.
func TestResolveMigratedPathIgnoresNewerDecoy(t *testing.T) {
	dir := t.TempDir()
	old := time.Now().Add(-72 * time.Hour)
	newer := time.Now().Add(1 * time.Hour)

	migrated := filepath.Join(dir, "20250101-real-note.md")
	writeNote(t, migrated, "20250101-120000", "Real Note", old)

	// A decoy with a strictly newer mtime — the mtime scan would have picked it.
	decoy := filepath.Join(dir, "zz-decoy.md")
	writeNote(t, decoy, "20250505-090000", "Decoy", newer)

	orig := filepath.Join(dir, "scratch.md")
	got := resolveMigratedPath(orig, "20250101-real-note.md", "20250101-120000")
	assert.Equal(t, migrated, got)
	assert.NotEqual(t, decoy, got, "the newest .md must never win")
}

// TestResolveMigratedPathFallsBackToStableID covers the collision case: the
// migrator appends a -N suffix when the standardized name is already taken, so
// a name match alone can point at the colliding file. The note's stable
// frontmatter id is authoritative and survives both moves and retitles.
func TestResolveMigratedPathFallsBackToStableID(t *testing.T) {
	dir := t.TempDir()

	// Someone else already owns the standardized name.
	collider := filepath.Join(dir, "20250101-real-note.md")
	writeNote(t, collider, "19990101-000000", "Someone Else", time.Now())

	// Our note landed on the suffixed name.
	migrated := filepath.Join(dir, "20250101-real-note-2.md")
	writeNote(t, migrated, "20250101-120000", "Real Note", time.Now().Add(-48*time.Hour))

	got := resolveMigratedPath(filepath.Join(dir, "scratch.md"), "20250101-real-note.md", "20250101-120000")
	assert.Equal(t, migrated, got, "id must beat a colliding name match")
}

// TestResolveMigratedPathNoAnswerReturnsOriginal: with nothing deterministic to
// go on, the original path comes back rather than a guess.
func TestResolveMigratedPathNoAnswerReturnsOriginal(t *testing.T) {
	dir := t.TempDir()
	writeNote(t, filepath.Join(dir, "unrelated.md"), "20250505-090000", "Unrelated", time.Now())

	orig := filepath.Join(dir, "scratch.md")
	assert.Equal(t, orig, resolveMigratedPath(orig, "", ""))
	assert.Equal(t, orig, resolveMigratedPath(orig, "20250101-missing.md", "20250101-120000"))
}

// TestApplyMigrationResolvesRenameDeterministically drives the real migration
// end to end with a decoy present, pinning that the resolved path is the
// migrated note and not whatever was written most recently.
func TestApplyMigrationResolvesRenameDeterministically(t *testing.T) {
	dir := t.TempDir()

	src := filepath.Join(dir, "scratch.md")
	writeNote(t, src, "20250101-120000", "Real Note", time.Now().Add(-72*time.Hour))

	final, err := applyMigration(src)
	require.NoError(t, err)

	require.NotEqual(t, src, final, "migration should have renamed the non-standard filename")
	assert.FileExists(t, final)
	assert.Equal(t, "20250101-120000", noteIDAt(final), "resolved file must be the note we migrated")

	// And the decoy trap: re-resolving with a newer .md present still picks by id.
	decoy := filepath.Join(dir, "zz-decoy.md")
	writeNote(t, decoy, "20250505-090000", "Decoy", time.Now().Add(1*time.Hour))
	assert.Equal(t, final, resolveMigratedPath(src, filepath.Base(final), "20250101-120000"))
}
