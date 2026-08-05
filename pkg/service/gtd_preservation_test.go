package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestUpdateNotePriorityPreservesGtd is the updater-path regression the gtd
// passthrough change ships with: the yaml.Node updater already preserves
// unknown keys, and this pins that a gtd block — nested waiting map included
// — survives UpdateNotePriority byte-visibly.
func TestUpdateNotePriorityPreservesGtd(t *testing.T) {
	tempDir := t.TempDir()
	notePath := filepath.Join(tempDir, "note.md")
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
	for _, want := range []string{"gtd:", "kind: task", "due: 2026-08-15", "pr: grovetools/nb#42"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("gtd content %q lost through UpdateNotePriority:\n%s", want, got)
		}
	}
}
