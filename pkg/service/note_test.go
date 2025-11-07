package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseNote(t *testing.T) {
	// Create a temporary test file
	tempDir := t.TempDir()

	// Create a nested structure to simulate the nb directory structure
	noteDir := filepath.Join(tempDir, "nb", "repos", "test-repo", "main", "current")
	err := os.MkdirAll(noteDir, 0755)
	require.NoError(t, err)

	notePath := filepath.Join(noteDir, "test-note.md")
	noteContent := `---
id: 20250111-test-note
title: Test Note
tags: [test, example, backend]
repository: test-repo
branch: main
created: 2025-01-11 10:00:00
modified: 2025-01-11 11:00:00
---

# Test Note

This is a test note with some content.

- [ ] A todo item
`

	err = os.WriteFile(notePath, []byte(noteContent), 0644)
	require.NoError(t, err)

	// Parse the note
	note, err := ParseNote(notePath)
	require.NoError(t, err)
	require.NotNil(t, note)

	// Verify all fields are populated correctly
	assert.Equal(t, notePath, note.Path)
	assert.Equal(t, "Test Note", note.Title)
	assert.Equal(t, "current", string(note.Type))
	assert.Equal(t, "test-repo", note.Workspace)
	assert.Equal(t, "main", note.Branch)
	assert.Equal(t, "test-repo", note.Repository)
	assert.Equal(t, "20250111-test-note", note.ID)
	assert.Equal(t, []string{"test", "example", "backend"}, note.Tags)
	assert.True(t, note.HasTodos)
	assert.False(t, note.IsArchived)
	assert.Greater(t, note.WordCount, 0)

	// Verify timestamps
	expectedCreated, _ := time.Parse("2006-01-02 15:04:05", "2025-01-11 10:00:00")
	expectedModified, _ := time.Parse("2006-01-02 15:04:05", "2025-01-11 11:00:00")
	assert.Equal(t, expectedCreated.Unix(), note.CreatedAt.Unix())
	assert.Equal(t, expectedModified.Unix(), note.ModifiedAt.Unix())
}

func TestParseNoteWithoutFrontmatter(t *testing.T) {
	// Create a temporary test file without frontmatter
	tempDir := t.TempDir()

	// Create a nested structure for global workspace
	noteDir := filepath.Join(tempDir, "nb", "global", "notes", "quick")
	err := os.MkdirAll(noteDir, 0755)
	require.NoError(t, err)

	notePath := filepath.Join(noteDir, "simple-note.md")
	noteContent := `# Simple Note

Just a simple note without frontmatter.
`

	err = os.WriteFile(notePath, []byte(noteContent), 0644)
	require.NoError(t, err)

	// Parse the note
	note, err := ParseNote(notePath)
	require.NoError(t, err)
	require.NotNil(t, note)

	// Verify fields are populated from path and content
	assert.Equal(t, notePath, note.Path)
	assert.Equal(t, "Simple Note", note.Title)
	assert.Equal(t, "quick", string(note.Type))
	assert.Equal(t, "global", note.Workspace)
	assert.Equal(t, "", note.Branch)
	assert.Empty(t, note.Tags)
	assert.False(t, note.HasTodos)
}
