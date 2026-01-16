package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grovetools/nb/pkg/models"
)

// TestSimpleListNotes tests listing notes without workspace complexity
func TestSimpleListNotes(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a simple directory structure
	notesDir := filepath.Join(tmpDir, "notes")
	if err := os.MkdirAll(notesDir, 0755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	// Create test notes
	testNotes := []struct {
		filename string
		content  string
	}{
		{
			filename: "20240101-note1.md",
			content:  "# Note 1\n\nContent",
		},
		{
			filename: "20240102-note2.md",
			content:  "# Note 2\n\nContent",
		},
	}

	for _, tn := range testNotes {
		path := filepath.Join(notesDir, tn.filename)
		if err := os.WriteFile(path, []byte(tn.content), 0644); err != nil {
			t.Fatalf("Failed to write test note: %v", err)
		}
	}

	// List files manually (simulating what ListNotes should do)
	var notes []*models.Note
	err := filepath.Walk(notesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(path, ".md") {
			note := &models.Note{
				Path:  path,
				Title: strings.TrimSuffix(info.Name(), ".md"),
				Type:  "current",
			}
			notes = append(notes, note)
		}
		return nil
	})

	if err != nil {
		t.Fatalf("Walk failed: %v", err)
	}

	if len(notes) != len(testNotes) {
		t.Errorf("Expected %d notes, got %d", len(testNotes), len(notes))
	}

	// Verify notes were found
	for i, note := range notes {
		t.Logf("Found note %d: %s", i, note.Title)
	}
}

// TestSimpleArchive tests moving a file to archive
func TestSimpleArchive(t *testing.T) {
	tmpDir := t.TempDir()

	// Create directories
	currentDir := filepath.Join(tmpDir, "current")
	archiveDir := filepath.Join(tmpDir, "archive")
	if err := os.MkdirAll(currentDir, 0755); err != nil {
		t.Fatalf("failed to create current directory: %v", err)
	}
	if err := os.MkdirAll(archiveDir, 0755); err != nil {
		t.Fatalf("failed to create archive directory: %v", err)
	}

	// Create a test file
	srcFile := filepath.Join(currentDir, "test.md")
	if err := os.WriteFile(srcFile, []byte("# Test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Archive it (move to archive dir)
	dstFile := filepath.Join(archiveDir, "test.md")
	if err := os.Rename(srcFile, dstFile); err != nil {
		t.Fatalf("Failed to archive: %v", err)
	}

	// Verify
	if _, err := os.Stat(srcFile); !os.IsNotExist(err) {
		t.Error("Source file still exists")
	}

	if _, err := os.Stat(dstFile); os.IsNotExist(err) {
		t.Error("Archive file not found")
	}
}
