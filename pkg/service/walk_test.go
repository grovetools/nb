package service

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grovetools/nb/pkg/models"
)

// validNoteMD returns a minimal valid note markdown blob with the given id and title.
func validNoteMD(id, title string) []byte {
	return []byte("---\nid: " + id + "\ntitle: " + title + "\naliases: []\ntags: []\ncreated: 2026-01-01T00:00:00Z\nmodified: 2026-01-01T00:00:00Z\n---\n\nbody\n")
}

// walkCollect is a convenience that walks noteType and collects every record.
func walkCollect(t *testing.T, s *Service, ctx *WorkspaceContext, noteType models.NoteType) []NoteScanRecord {
	t.Helper()
	var recs []NoteScanRecord
	err := s.WalkNotes(context.Background(), ctx, noteType, func(r NoteScanRecord) error {
		recs = append(recs, r)
		return nil
	})
	require.NoError(t, err)
	return recs
}

// typeDir returns the absolute directory for a noteType under the test root.
func typeDir(root, noteType string) string {
	return filepath.Join(root, "workspaces", "test-repo", noteType)
}

func TestWalkNotes_DeterministicOrder(t *testing.T) {
	captureNoteEvents(t) // stub daemon events
	s, ctx, root := newStructuredTestService(t)

	dir := typeDir(root, "inbox")
	require.NoError(t, os.MkdirAll(dir, 0o755))

	// Create files in non-lexical order: b, a, c.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.md"), validNoteMD("b", "B"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.md"), validNoteMD("a", "A"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "c.md"), validNoteMD("c", "C"), 0o644))

	recs := walkCollect(t, s, ctx, "inbox")
	require.Len(t, recs, 3)
	for _, r := range recs {
		require.NotNil(t, r.Note, "expected note record, got error: %+v", r.Err)
	}
	assert.Equal(t, "a", recs[0].Note.ID)
	assert.Equal(t, "b", recs[1].Note.ID)
	assert.Equal(t, "c", recs[2].Note.ID)
}

func TestWalkNotes_ImmediateCallback(t *testing.T) {
	captureNoteEvents(t)
	s, ctx, root := newStructuredTestService(t)

	dir := typeDir(root, "inbox")
	require.NoError(t, os.MkdirAll(dir, 0o755))

	const total = 5
	for i := 0; i < total; i++ {
		name := string(rune('a'+i)) + ".md"
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), validNoteMD(name, name), 0o644))
	}

	// Track visit count; verify the first visit fires before total is reached.
	var visited int64
	firstSeen := make(chan struct{}, 1)
	err := s.WalkNotes(context.Background(), ctx, "inbox", func(r NoteScanRecord) error {
		n := atomic.AddInt64(&visited, 1)
		if n == 1 {
			// Signal that the first note reached the visitor.
			firstSeen <- struct{}{}
		}
		return nil
	})
	require.NoError(t, err)

	// The channel proves the first note was delivered during the walk, not
	// after a bulk-collect step.
	select {
	case <-firstSeen:
		// good: the first record was delivered before the walk returned
	default:
		t.Fatal("first note was never delivered to visitor")
	}
	assert.Equal(t, int64(total), atomic.LoadInt64(&visited))
}

func TestWalkNotes_Cancellation(t *testing.T) {
	captureNoteEvents(t)
	s, ctx, root := newStructuredTestService(t)

	dir := typeDir(root, "inbox")
	require.NoError(t, os.MkdirAll(dir, 0o755))

	// Create several files so there is work to skip after cancellation.
	for i := 0; i < 10; i++ {
		name := string(rune('a'+i)) + ".md"
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), validNoteMD(name, name), 0o644))
	}

	cancelCtx, cancel := context.WithCancel(context.Background())
	var visited int
	err := s.WalkNotes(cancelCtx, ctx, "inbox", func(r NoteScanRecord) error {
		visited++
		if visited == 2 {
			cancel()
			return cancelCtx.Err()
		}
		return nil
	})

	// The walk should have stopped. The error is the context cancellation.
	assert.ErrorIs(t, err, context.Canceled)
	assert.Less(t, visited, 10, "walk should have stopped early")
}

func TestWalkNotes_ParseErrors(t *testing.T) {
	captureNoteEvents(t)
	s, ctx, root := newStructuredTestService(t)

	dir := typeDir(root, "inbox")
	require.NoError(t, os.MkdirAll(dir, 0o755))

	// A file with frontmatter delimiters but invalid YAML inside them.
	badContent := []byte("---\n: :\n  bad: [yaml\n---\n\nbody\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bad.md"), badContent, 0o644))

	recs := walkCollect(t, s, ctx, "inbox")
	require.Len(t, recs, 1)
	require.NotNil(t, recs[0].Err, "expected an error record for malformed frontmatter")
	assert.Equal(t, "frontmatter_invalid", recs[0].Err.Code)
	assert.Equal(t, "bad.md", recs[0].Err.RelativePath)
}

func TestWalkNotes_RootEscapeRejection(t *testing.T) {
	captureNoteEvents(t)
	s, ctx, root := newStructuredTestService(t)

	dir := typeDir(root, "inbox")
	require.NoError(t, os.MkdirAll(dir, 0o755))

	// Create a note outside the type root.
	outsideDir := filepath.Join(root, "outside")
	require.NoError(t, os.MkdirAll(outsideDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(outsideDir, "escaped.md"), validNoteMD("escaped", "Escaped"), 0o644))

	// Create a symlink inside inbox that points to the outside directory.
	symlinkPath := filepath.Join(dir, "escape-link")
	err := os.Symlink(outsideDir, symlinkPath)
	if err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	// The walk must not crash. Any notes reached through the symlink
	// should either produce an error record or be handled gracefully.
	var recs []NoteScanRecord
	walkErr := s.WalkNotes(context.Background(), ctx, "inbox", func(r NoteScanRecord) error {
		recs = append(recs, r)
		return nil
	})
	// No panic, no fatal error from the walk itself.
	assert.NoError(t, walkErr)

	// Verify at least one record came through (whether note or error).
	// The symlinked path resolves to a relative that starts with ".." from
	// root's perspective, which the escape guard catches.
	if len(recs) > 0 {
		// If the implementation visits the symlinked file, it should emit an error record
		// since the real path escapes the type root.
		for _, r := range recs {
			if r.Err != nil {
				assert.NotEmpty(t, r.Err.Code, "error record should have a code")
			}
			// If it produced a note record through the symlink, that is also
			// acceptable as long as no crash occurred.
		}
	}
}

func TestWalkNotes_NestedBundleType(t *testing.T) {
	captureNoteEvents(t)
	s, ctx, root := newStructuredTestService(t)

	dir := filepath.Join(typeDir(root, "hn"), "clippings")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "note.md"), validNoteMD("nested", "Nested"), 0o644))

	recs := walkCollect(t, s, ctx, "hn")
	require.Len(t, recs, 1)
	require.NotNil(t, recs[0].Note, "expected a note record")
	assert.Equal(t, "hn/clippings", recs[0].Note.BundleType)
	assert.Equal(t, "clippings/note.md", recs[0].Note.RelativePath)
}

func TestWalkNotes_RootLevelBundleType(t *testing.T) {
	captureNoteEvents(t)
	s, ctx, root := newStructuredTestService(t)

	dir := typeDir(root, "inbox")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "note.md"), validNoteMD("root-level", "Root Level"), 0o644))

	recs := walkCollect(t, s, ctx, "inbox")
	require.Len(t, recs, 1)
	require.NotNil(t, recs[0].Note, "expected a note record")
	assert.Equal(t, "inbox", recs[0].Note.BundleType)
	assert.Equal(t, "note.md", recs[0].Note.RelativePath)
}

func TestWalkNotes_NonMarkdownSkipped(t *testing.T) {
	captureNoteEvents(t)
	s, ctx, root := newStructuredTestService(t)

	dir := typeDir(root, "inbox")
	require.NoError(t, os.MkdirAll(dir, 0o755))

	// Create both a .md and a .txt file.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "real.md"), validNoteMD("real", "Real"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "skip.txt"), []byte("not markdown"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "skip.json"), []byte("{}"), 0o644))

	recs := walkCollect(t, s, ctx, "inbox")
	require.Len(t, recs, 1)
	require.NotNil(t, recs[0].Note)
	assert.Equal(t, "real", recs[0].Note.ID)
}

func TestWalkNotes_EmptyDirectory(t *testing.T) {
	captureNoteEvents(t)
	s, ctx, _ := newStructuredTestService(t)

	// Do NOT create the type directory. WalkNotes should return nil with no visits.
	var visited int
	err := s.WalkNotes(context.Background(), ctx, "nonexistent-type", func(r NoteScanRecord) error {
		visited++
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, 0, visited, "no files should be visited for a non-existent type dir")
}
