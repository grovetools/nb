package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsValidPriority(t *testing.T) {
	cases := map[string]bool{
		"":   true, // empty clears the field
		"p0": true,
		"p1": true,
		"p2": true,
		"p3": true,
		"p4": false,
		"P0": false,
		"0":  false,
		"x":  false,
	}
	for in, want := range cases {
		if got := IsValidPriority(in); got != want {
			t.Errorf("IsValidPriority(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestUpdateNotePriority(t *testing.T) {
	tempDir := t.TempDir()
	notePath := filepath.Join(tempDir, "note.md")
	content := `---
id: 20250111-test
title: Test Note
aliases: []
tags: [test]
created: 2025-01-11 10:00:00
modified: 2025-01-11 11:00:00
---

# Test Note
`
	require.NoError(t, os.WriteFile(notePath, []byte(content), 0o644))

	s := &Service{}

	// Invalid priority is rejected.
	require.Error(t, s.UpdateNotePriority(notePath, "p9"))

	// Set a priority and verify it parses back.
	require.NoError(t, s.UpdateNotePriority(notePath, "p0"))
	note, err := ParseNote(notePath)
	require.NoError(t, err)
	assert.Equal(t, "p0", note.Priority)

	// Change the priority.
	require.NoError(t, s.UpdateNotePriority(notePath, "p2"))
	note, err = ParseNote(notePath)
	require.NoError(t, err)
	assert.Equal(t, "p2", note.Priority)

	// Clear the priority.
	require.NoError(t, s.UpdateNotePriority(notePath, ""))
	note, err = ParseNote(notePath)
	require.NoError(t, err)
	assert.Equal(t, "", note.Priority)
}
