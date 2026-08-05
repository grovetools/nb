package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestUpdateNotePriorityPreservesPluginBlock covers the OTHER frontmatter
// write path. UpdateNotePriority edits the yaml.Node document in place rather
// than going through Parse→Build, so it never had the unknown-key drop — but
// nothing pinned that, and it is the path a plugin's block is most likely to
// meet (priority is bumped from the browser while gtd owns its own block).
//
// The date assertion is the load-bearing one: an in-place node edit must leave
// `due: 2026-08-15` spelled exactly that way.
func TestUpdateNotePriorityPreservesPluginBlock(t *testing.T) {
	notePath := filepath.Join(t.TempDir(), "note.md")
	content := `---
id: 20260805-test
title: Test Note
aliases: []
tags: [test]
created: 2026-08-05 10:00:00
modified: 2026-08-05 11:00:00
gtd:
  kind: task
  due: 2026-08-15
  waiting:
    pr: grovetools/nb#42
---

# Test Note
`
	require.NoError(t, os.WriteFile(notePath, []byte(content), 0o644))

	s := &Service{}
	require.NoError(t, s.UpdateNotePriority(notePath, "p1"))

	got, err := os.ReadFile(notePath)
	require.NoError(t, err)
	for _, want := range []string{
		"priority: p1",
		"gtd:",
		"  kind: task",
		"  due: 2026-08-15",
		"  waiting:",
		"    pr: grovetools/nb#42",
	} {
		if !strings.Contains(string(got), want) {
			t.Errorf("%q lost or rewritten by UpdateNotePriority:\n%s", want, got)
		}
	}
	if strings.Contains(string(got), "00:00:00 +0000 UTC") || strings.Contains(string(got), "2026-08-15T00:00:00Z") {
		t.Errorf("plugin date coerced by UpdateNotePriority:\n%s", got)
	}
}

// TestRenamePreservesPluginBlock is the Parse→Build path — the one that
// actually dropped unknown keys before Extra existed. Renaming is the most
// ordinary thing a user does to a note whose metadata nb does not own.
func TestRenamePreservesPluginBlock(t *testing.T) {
	notePath := filepath.Join(t.TempDir(), "tracked-project.md")
	content := `---
id: 20260805-tracked
title: Tracked Project
aliases: []
tags: [projects]
created: 2026-08-05 10:00:00
modified: 2026-08-05 10:00:00
gtd:
  kind: project
  status: active
  due: 2026-08-15
  waiting:
    pr: grovetools/nb#42
---

# Tracked Project
`
	require.NoError(t, os.WriteFile(notePath, []byte(content), 0o644))

	s := &Service{}
	newPath, err := s.RenameNote(notePath, "Tracked Project Renamed")
	require.NoError(t, err)

	after, err := os.ReadFile(newPath)
	require.NoError(t, err)
	for _, want := range []string{
		"title: Tracked Project Renamed",
		"gtd:",
		"  kind: project",
		"  status: active",
		"  due: 2026-08-15",
		"    pr: grovetools/nb#42",
	} {
		if !strings.Contains(string(after), want) {
			t.Errorf("%q lost or rewritten by rename:\n%s", want, after)
		}
	}
	if strings.Contains(string(after), "00:00:00 +0000 UTC") || strings.Contains(string(after), "2026-08-15T00:00:00Z") {
		t.Errorf("plugin date coerced by rename:\n%s", after)
	}
}
