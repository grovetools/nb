package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	coremodels "github.com/grovetools/core/pkg/models"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grovetools/nb/pkg/frontmatter"
)

// captureNoteEvents replaces the fire-and-forget daemon notifier with a
// synchronous recorder for the duration of a test.
func captureNoteEvents(t *testing.T) *[]coremodels.NoteEvent {
	t.Helper()
	events := &[]coremodels.NoteEvent{}
	orig := notifyDaemonNoteEvent
	notifyDaemonNoteEvent = func(e coremodels.NoteEvent) {
		*events = append(*events, e)
	}
	t.Cleanup(func() { notifyDaemonNoteEvent = orig })
	return events
}

func newTestService() *Service {
	logger := logrus.New()
	logger.SetOutput(os.Stderr)
	logger.SetLevel(logrus.PanicLevel)
	return &Service{Logger: logrus.NewEntry(logger)}
}

func TestArchiveNotesEmitsTypedMoveEvent(t *testing.T) {
	events := captureNoteEvents(t)
	s := newTestService()

	noteDir := filepath.Join(t.TempDir(), "nb", "repos", "test-repo", "main", "inbox")
	require.NoError(t, os.MkdirAll(noteDir, 0o755))
	notePath := filepath.Join(noteDir, "task.md")
	require.NoError(t, os.WriteFile(notePath, []byte("# Task\n"), 0o644))

	require.NoError(t, s.ArchiveNotes(nil, []string{notePath}))

	archivedPath := filepath.Join(noteDir, ".archive", "task.md")
	assert.NoFileExists(t, notePath)
	assert.FileExists(t, archivedPath)

	require.Len(t, *events, 1)
	ev := (*events)[0]
	assert.Equal(t, coremodels.NoteEventArchived, ev.Event)
	// Path must be the new .archive location and PrevPath the original:
	// the daemon's rename detection requires both sides of the move.
	assert.Equal(t, archivedPath, ev.Path)
	assert.Equal(t, notePath, ev.PrevPath)
	assert.Equal(t, ev.Workspace, ev.PrevWorkspace)
	assert.Equal(t, ev.NoteType, ev.PrevNoteType)
}

func TestRenameNoteEmitsTypedRenameEvent(t *testing.T) {
	events := captureNoteEvents(t)
	s := newTestService()

	noteDir := filepath.Join(t.TempDir(), "nb", "repos", "test-repo", "main", "inbox")
	require.NoError(t, os.MkdirAll(noteDir, 0o755))
	oldPath := filepath.Join(noteDir, "old-title.md")
	noteContent := `---
id: 20250111-old-title
title: Old Title
aliases: []
tags: []
created: 2025-01-11 10:00:00
modified: 2025-01-11 11:00:00
---

# Old Title

Body.
`
	require.NoError(t, os.WriteFile(oldPath, []byte(noteContent), 0o644))

	newPath, err := s.RenameNote(oldPath, "New Title")
	require.NoError(t, err)
	require.NotEqual(t, oldPath, newPath)

	require.Len(t, *events, 1)
	ev := (*events)[0]
	assert.Equal(t, coremodels.NoteEventRenamed, ev.Event)
	assert.Equal(t, newPath, ev.Path)
	assert.Equal(t, oldPath, ev.PrevPath)

	// The rewritten frontmatter keeps the legacy created timestamp untouched
	// but writes the new modified timestamp in RFC3339/UTC.
	content, err := os.ReadFile(newPath)
	require.NoError(t, err)
	fm, _, err := frontmatter.Parse(string(content))
	require.NoError(t, err)
	require.NotNil(t, fm)
	assert.Equal(t, "2025-01-11 10:00:00", fm.Created)
	assert.True(t, strings.HasSuffix(fm.Modified, "Z"), "modified should be UTC RFC3339, got %q", fm.Modified)
	_, err = frontmatter.ParseTimestamp(fm.Modified)
	assert.NoError(t, err)
}
