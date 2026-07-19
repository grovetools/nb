package migration

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grovetools/nb/pkg/frontmatter"
)

// TestMigrationLeavesWellFormedIDUntouched pins that a note whose frontmatter
// already carries a valid, load-bearing id survives migration byte-identical.
// The analyzer's idPattern is a prefix match (^\d{8}-\d{6}), so a well-formed
// id like 20250101-120000-valid-note matches and yields no invalid_id issue;
// with no other issues the file is skipped and never rewritten.
func TestMigrationLeavesWellFormedIDUntouched(t *testing.T) {
	basePath := t.TempDir()

	// Placed directly in basePath so no directory-derived tags are expected.
	notePath := filepath.Join(basePath, "20250101-valid-note.md")
	content := `---
id: 20250101-120000-valid-note
title: Valid Note
aliases: []
tags: []
created: 2025-01-01 10:00:00
modified: 2025-01-01 11:00:00
---

# Valid Note

Body content.
`
	require.NoError(t, os.WriteFile(notePath, []byte(content), 0o644))

	// Analyzer must not flag the id.
	issues, err := AnalyzeFile(notePath, basePath)
	require.NoError(t, err)
	for _, issue := range issues {
		assert.NotEqual(t, "id", issue.Field, "well-formed id must not be flagged: %+v", issue)
		assert.NotEqual(t, "invalid_id", issue.Type, "well-formed id must not be flagged: %+v", issue)
	}

	// End-to-end migration must leave the bytes untouched.
	opts := MigrationOptions{Scope: MigrationScope{All: true}, NoBackup: true}
	_, err = Migrate(basePath, opts, &bytes.Buffer{}, nil)
	require.NoError(t, err)

	after, err := os.ReadFile(notePath)
	require.NoError(t, err)
	assert.Equal(t, content, string(after), "well-formed note must pass through byte-identical")
}

// TestMigrationBackfillsIDAcrossLifecycleDirs verifies the id backfill reaches
// notes in all four lifecycle dirs plus .archive/. Migrate walks the base path
// recursively with no directory filtering, so every *.md is analyzed regardless
// of which lifecycle stage it lives in.
func TestMigrationBackfillsIDAcrossLifecycleDirs(t *testing.T) {
	basePath := t.TempDir()

	dirs := []string{"inbox", "in_progress", "review", "completed", ".archive"}
	notePaths := make(map[string]string, len(dirs))

	for _, dir := range dirs {
		noteDir := filepath.Join(basePath, dir)
		require.NoError(t, os.MkdirAll(noteDir, 0o755))
		// Filename matches the standard for its (missing) id so only the id is
		// backfilled and the file is not renamed.
		notePath := filepath.Join(noteDir, "20250101-"+sanitizeFilename(dir)+"-note.md")
		content := "---\n" +
			"title: " + dir + " Note\n" +
			"tags: [" + dir + "]\n" +
			"created: 2025-01-01 10:00:00\n" +
			"modified: 2025-01-01 11:00:00\n" +
			"---\n\n# " + dir + " Note\n\nBody.\n"
		require.NoError(t, os.WriteFile(notePath, []byte(content), 0o644))
		notePaths[dir] = notePath

		// Precondition: no id yet.
		fm, _, err := frontmatter.Parse(content)
		require.NoError(t, err)
		require.NotNil(t, fm)
		require.Empty(t, fm.ID)
	}

	opts := MigrationOptions{Scope: MigrationScope{All: true}, NoBackup: true}
	_, err := Migrate(basePath, opts, &bytes.Buffer{}, nil)
	require.NoError(t, err)

	for _, dir := range dirs {
		after, err := os.ReadFile(notePaths[dir])
		require.NoError(t, err, "note in %s should remain at its path", dir)
		fm, _, err := frontmatter.Parse(string(after))
		require.NoError(t, err)
		require.NotNil(t, fm)
		assert.NotEmpty(t, fm.ID, "id should be backfilled in %s", dir)
		assert.True(t, idPattern.MatchString(fm.ID),
			"backfilled id in %s should be well-formed, got %q", dir, fm.ID)
	}
}
