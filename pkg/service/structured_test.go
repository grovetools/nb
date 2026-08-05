package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	coreconfig "github.com/grovetools/core/config"
	coremodels "github.com/grovetools/core/pkg/models"
	coreworkspace "github.com/grovetools/core/pkg/workspace"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grovetools/nb/pkg/frontmatter"
)

// producerFields builds ProducerFields from a literal map so these tests keep
// reading like the argument vectors a producer actually sends, while going
// through the same node encoding the --frontmatter-file path uses.
func producerFields(t *testing.T, fields map[string]any) frontmatter.ProducerFields {
	t.Helper()
	pf, err := frontmatter.NewProducerFields(fields)
	require.NoError(t, err)
	return pf
}

// newStructuredTestService builds a Service against a throwaway centralized
// notebook root, plus a workspace context that resolves into it. Unlike
// newTestService this wires a real NotebookLocator, because the structured
// paths resolve type directories through it.
func newStructuredTestService(t *testing.T) (*Service, *WorkspaceContext, string) {
	t.Helper()
	root := t.TempDir()
	cfg := &coreconfig.Config{
		Notebooks: &coreconfig.NotebooksConfig{
			Definitions: map[string]*coreconfig.Notebook{"main": {RootDir: root}},
			Rules:       &coreconfig.NotebookRules{Default: "main"},
		},
	}
	logger := logrus.New()
	logger.SetOutput(os.Stderr)
	logger.SetLevel(logrus.PanicLevel)
	svc, err := New(&Config{}, coreworkspace.NewProviderFromNodes(nil), cfg, logrus.NewEntry(logger))
	require.NoError(t, err)

	ctx := &WorkspaceContext{
		NotebookContextWorkspace: &coreworkspace.WorkspaceNode{Name: "test-repo", NotebookName: "main"},
		Branch:                   "main",
	}
	return svc, ctx, root
}

// readFrontmatter parses a note file and requires it to have frontmatter.
func readFrontmatter(t *testing.T, path string) (*frontmatter.Frontmatter, string) {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	fm, body, err := frontmatter.Parse(string(content))
	require.NoError(t, err)
	require.NotNil(t, fm, "note %s has no frontmatter", path)
	return fm, body
}

func TestCreateStructuredNoteMergesProducerFrontmatter(t *testing.T) {
	events := captureNoteEvents(t)
	s, ctx, root := newStructuredTestService(t)

	producer := map[string]any{
		"title":             "Pomodoro 2026-08-05 09:00–09:50",
		"tags":              []any{"pomodoro", "work-block"},
		"pomodoro_block_id": "blk-1",
		"id":                "forged", // nb-owned: must lose
	}
	note, existed, err := s.CreateStructuredNote(ctx, "worklog/pomodoro", "Pomodoro Block", producerFields(t, producer), "# Work-block summary\n", StructuredNoteOptions{
		IdempotencyKey: "pomodoro:blk-1",
	})
	require.NoError(t, err)
	assert.False(t, existed)

	// Nested type resolves to a nested directory under the workspace.
	wantDir := filepath.Join(root, "workspaces", "test-repo", "worklog", "pomodoro")
	assert.Equal(t, wantDir, filepath.Dir(note.Path))

	fm, body := readFrontmatter(t, note.Path)
	assert.NotEqual(t, "forged", fm.ID, "producer must not forge the note id")
	assert.NotEmpty(t, fm.ID)
	assert.Equal(t, "Pomodoro 2026-08-05 09:00–09:50", fm.Title)
	assert.Equal(t, []string{"pomodoro", "work-block"}, fm.Tags)
	assert.Equal(t, "blk-1", fm.ExtraValue("pomodoro_block_id"))
	assert.Equal(t, "pomodoro:blk-1", fm.ExtraValue(frontmatter.IdempotencyKeyField))
	assert.NotEmpty(t, fm.Created)
	assert.Contains(t, body, "# Work-block summary")

	// The creation went through the EmitNoteEvent funnel.
	require.Len(t, *events, 1)
	ev := (*events)[0]
	assert.Equal(t, coremodels.NoteEventCreated, ev.Event)
	assert.Equal(t, note.Path, ev.Path)
	assert.Equal(t, "worklog/pomodoro", ev.NoteType)
	assert.Equal(t, "test-repo", ev.Workspace)
}

// TestCreateStructuredNoteIdempotent is the requirement that IS the ticket: a
// repeated create with the same key in the same resolved type dir returns the
// existing note's receipt — no second file, no second created event.
func TestCreateStructuredNoteIdempotent(t *testing.T) {
	events := captureNoteEvents(t)
	s, ctx, _ := newStructuredTestService(t)

	opts := StructuredNoteOptions{IdempotencyKey: "pomodoro:blk-1"}
	first, existed, err := s.CreateStructuredNote(ctx, "worklog/pomodoro", "Block One", producerFields(t, map[string]any{"pomodoro_block_id": "blk-1"}), "body v1", opts)
	require.NoError(t, err)
	require.False(t, existed)

	// Different title, body, and producer fields: the KEY decides identity.
	second, existed, err := s.CreateStructuredNote(ctx, "worklog/pomodoro", "Retried Block", producerFields(t, map[string]any{"pomodoro_block_id": "blk-1-retry"}), "body v2", opts)
	require.NoError(t, err)
	assert.True(t, existed)
	assert.Equal(t, first.Path, second.Path)
	assert.Equal(t, first.ID, second.ID)

	entries, err := os.ReadDir(filepath.Dir(first.Path))
	require.NoError(t, err)
	assert.Len(t, entries, 1, "the retry must not create a second file")

	// The retry wrote nothing, so its content is still v1.
	_, body := readFrontmatter(t, first.Path)
	assert.Contains(t, body, "body v1")

	require.Len(t, *events, 1, "only the first create may emit an event")

	// A DIFFERENT key in the same dir is a different note.
	third, existed, err := s.CreateStructuredNote(ctx, "worklog/pomodoro", "Block Two", nil, "", StructuredNoteOptions{IdempotencyKey: "pomodoro:blk-2"})
	require.NoError(t, err)
	assert.False(t, existed)
	assert.NotEqual(t, first.Path, third.Path)
}

func TestCreateStructuredNoteFilename(t *testing.T) {
	captureNoteEvents(t)
	s, ctx, root := newStructuredTestService(t)

	note, _, err := s.CreateStructuredNote(ctx, "hn/clippings", "Some Article", nil, "body", StructuredNoteOptions{Filename: "article.md"})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "workspaces", "test-repo", "hn", "clippings", "article.md"), note.Path)

	// The exact producer-chosen name colliding without a matching key is an
	// error, never a silent overwrite or a silently different filename.
	_, _, err = s.CreateStructuredNote(ctx, "hn/clippings", "Other Article", nil, "other", StructuredNoteOptions{Filename: "article.md"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")

	// Validation: relative basename only, must end .md. (An empty value is
	// "no --filename", i.e. the generated-name path, so it is not in this
	// list; ValidateNoteFilename itself rejects it, see its test.)
	for _, bad := range []string{"sub/dir.md", `back\slash.md`, "../escape.md", "note.txt", ".md"} {
		_, _, err := s.CreateStructuredNote(ctx, "inbox", "T", nil, "", StructuredNoteOptions{Filename: bad})
		assert.Error(t, err, "filename %q must be rejected", bad)
	}
}

// TestCreateStructuredNoteGeneratedNameCollision: same title on the same day
// without keys must yield two files, not one overwritten file.
func TestCreateStructuredNoteGeneratedNameCollision(t *testing.T) {
	captureNoteEvents(t)
	s, ctx, _ := newStructuredTestService(t)

	first, _, err := s.CreateStructuredNote(ctx, "inbox", "Same Title", nil, "first", StructuredNoteOptions{})
	require.NoError(t, err)
	second, _, err := s.CreateStructuredNote(ctx, "inbox", "Same Title", nil, "second", StructuredNoteOptions{})
	require.NoError(t, err)

	assert.NotEqual(t, first.Path, second.Path)
	_, body := readFrontmatter(t, first.Path)
	assert.Contains(t, body, "first", "the first note must not be overwritten")
}

// TestMoveCopyPreservesProducerKeys pins the prerequisite fix: the move/copy
// frontmatter rewrite (updateNoteFrontmatter, a Parse→Build path) used to
// strip every key the Frontmatter struct had no field for. This drives the
// exact code path transferNotes uses.
func TestMoveCopyPreservesProducerKeys(t *testing.T) {
	s := newTestService()

	noteDir := filepath.Join(t.TempDir(), "nb", "repos", "test-repo", "main", "in_progress")
	require.NoError(t, os.MkdirAll(noteDir, 0o755))
	notePath := filepath.Join(noteDir, "block.md")
	noteContent := `---
id: 20260805-090000-block
title: Pomodoro Block
aliases: []
tags: [pomodoro]
created: 2026-08-05T09:00:00Z
modified: 2026-08-05T09:50:00Z
idempotency_key: pomodoro:blk-1
pomodoro_block_id: blk-1
pomodoro_jobs_completed: 3
---

# Pomodoro Block
`
	require.NoError(t, os.WriteFile(notePath, []byte(noteContent), 0o644))

	require.NoError(t, s.updateNoteFrontmatter(notePath, nil, "in_progress", false))

	fm, _ := readFrontmatter(t, notePath)
	assert.Equal(t, "20260805-090000-block", fm.ID)
	assert.Equal(t, "blk-1", fm.ExtraValue("pomodoro_block_id"), "move rewrite must keep producer keys")
	assert.Equal(t, 3, fm.ExtraValue("pomodoro_jobs_completed"))
	assert.Equal(t, "pomodoro:blk-1", fm.ExtraValue(frontmatter.IdempotencyKeyField))
}

func TestUpdateStructuredNoteMergePolicy(t *testing.T) {
	events := captureNoteEvents(t)
	s, ctx, _ := newStructuredTestService(t)

	note, _, err := s.CreateStructuredNote(ctx, "worklog/pomodoro", "Block", producerFields(t, map[string]any{
		"pomodoro_block_id":       "blk-1",
		"pomodoro_summary_status": "pending",
	}), "# v1\n", StructuredNoteOptions{IdempotencyKey: "pomodoro:blk-1"})
	require.NoError(t, err)
	fmBefore, _ := readFrontmatter(t, note.Path)

	newBody := "# v2\n\nThe LLM summary arrived.\n"
	updated, err := s.UpdateStructuredNote(note.Path, producerFields(t, map[string]any{
		"pomodoro_summary_status": "final",
		"pomodoro_tokens":         1234,
		"id":                      "forged",               // nb-owned: must lose
		"created":                 "1999-01-01T00:00:00Z", // nb-owned: must lose
	}), &newBody)
	require.NoError(t, err)
	// UpdateStructuredNote canonicalizes the path (symlinks, e.g. macOS
	// /var → /private/var), so compare canonical spellings.
	wantPath, err := filepath.EvalSymlinks(note.Path)
	require.NoError(t, err)
	assert.Equal(t, wantPath, updated.Path)

	fm, body := readFrontmatter(t, note.Path)
	// nb-owned identity preserved, modified refreshed.
	assert.Equal(t, fmBefore.ID, fm.ID)
	assert.Equal(t, fmBefore.Created, fm.Created)
	assert.True(t, strings.HasSuffix(fm.Modified, "Z"), "modified should be RFC3339 UTC, got %q", fm.Modified)
	// Producer fields merged: updated ones replaced, untouched ones kept.
	assert.Equal(t, "final", fm.ExtraValue("pomodoro_summary_status"))
	assert.Equal(t, 1234, fm.ExtraValue("pomodoro_tokens"))
	assert.Equal(t, "blk-1", fm.ExtraValue("pomodoro_block_id"))
	assert.Equal(t, "pomodoro:blk-1", fm.ExtraValue(frontmatter.IdempotencyKeyField))
	// Body replaced.
	assert.Contains(t, body, "# v2")
	assert.NotContains(t, body, "# v1")

	// create + update, both through the funnel.
	require.Len(t, *events, 2)
	assert.Equal(t, coremodels.NoteEventUpdated, (*events)[1].Event)
	assert.Equal(t, updated.Path, (*events)[1].Path)
}

// TestUpdateStructuredNoteKeepsBodyWhenNil: no --body-file means the body is
// untouched, not blanked.
func TestUpdateStructuredNoteKeepsBodyWhenNil(t *testing.T) {
	captureNoteEvents(t)
	s, ctx, _ := newStructuredTestService(t)

	note, _, err := s.CreateStructuredNote(ctx, "inbox", "Keep Body", nil, "# The body stays\n", StructuredNoteOptions{})
	require.NoError(t, err)

	_, err = s.UpdateStructuredNote(note.Path, producerFields(t, map[string]any{"custom_field": "x"}), nil)
	require.NoError(t, err)

	fm, body := readFrontmatter(t, note.Path)
	assert.Equal(t, "x", fm.ExtraValue("custom_field"))
	assert.Contains(t, body, "# The body stays")
}

func TestUpdateStructuredNoteRefusesForeignPaths(t *testing.T) {
	captureNoteEvents(t)
	s, _, _ := newStructuredTestService(t)

	// Relative paths are refused outright.
	_, err := s.UpdateStructuredNote("relative/note.md", nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "absolute")

	// An absolute path outside every notebook root is refused, even if a
	// perfectly valid note lives there.
	outside := filepath.Join(t.TempDir(), "note.md")
	require.NoError(t, os.WriteFile(outside, []byte("---\nid: x\ntitle: X\naliases: []\ntags: []\ncreated: 2026-01-01T00:00:00Z\nmodified: 2026-01-01T00:00:00Z\n---\n\nBody\n"), 0o644))
	_, err = s.UpdateStructuredNote(outside, producerFields(t, map[string]any{"k": "v"}), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "notebook root")
}

// TestUpdateStructuredNoteAcceptsMarkerNotebook covers the fallback for
// notebooks that exist on disk but not in config: the notebook.yml marker.
func TestUpdateStructuredNoteAcceptsMarkerNotebook(t *testing.T) {
	captureNoteEvents(t)
	s, _, _ := newStructuredTestService(t)

	markerRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(markerRoot, "notebook.yml"), []byte("name: adhoc\n"), 0o644))
	notePath := filepath.Join(markerRoot, "inbox", "note.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(notePath), 0o755))
	require.NoError(t, os.WriteFile(notePath, []byte("---\nid: m1\ntitle: Marked\naliases: []\ntags: []\ncreated: 2026-01-01T00:00:00Z\nmodified: 2026-01-01T00:00:00Z\n---\n\nBody\n"), 0o644))

	updated, err := s.UpdateStructuredNote(notePath, producerFields(t, map[string]any{"source": "test"}), nil)
	require.NoError(t, err)
	fm, _ := readFrontmatter(t, updated.Path)
	assert.Equal(t, "test", fm.ExtraValue("source"))
}

// TestUpdateStructuredNoteRefusesFrontmatterless: updating a file without
// frontmatter would mean inventing an identity — that is `nb new`'s job.
func TestUpdateStructuredNoteRefusesFrontmatterless(t *testing.T) {
	captureNoteEvents(t)
	s, ctx, _ := newStructuredTestService(t)

	// Place a bare file inside a legitimate notebook root.
	note, _, err := s.CreateStructuredNote(ctx, "inbox", "Anchor", nil, "", StructuredNoteOptions{})
	require.NoError(t, err)
	bare := filepath.Join(filepath.Dir(note.Path), "bare.md")
	require.NoError(t, os.WriteFile(bare, []byte("# Just markdown\n"), 0o644))

	_, err = s.UpdateStructuredNote(bare, producerFields(t, map[string]any{"k": "v"}), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no frontmatter")
}

func TestValidateNoteFilename(t *testing.T) {
	assert.NoError(t, ValidateNoteFilename("article.md"))
	assert.NoError(t, ValidateNoteFilename("2026-08-05-block.md"))
	for _, bad := range []string{"", "a/b.md", `a\b.md`, "..", "article.txt", ".md", "../up.md"} {
		assert.Error(t, ValidateNoteFilename(bad), "want rejection for %q", bad)
	}
}
