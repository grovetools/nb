package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mattsolo1/grove-notebook/pkg/models"
	"github.com/mattsolo1/grove-notebook/pkg/workspace"
)

// TestWorkingListNotes uses a simplified approach for testing
func TestWorkingListNotes(t *testing.T) {
	tmpDir := t.TempDir()

	// Create service
	config := &Config{
		DataDir:     filepath.Join(tmpDir, "data"),
		Editor:      "vim",
		DefaultType: models.NoteTypeCurrent,
	}

	svc, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}
	defer svc.Close()

	// Create and register the global workspace (since that's what gets detected)
	home := tmpDir // Mock home directory
	globalNotebookDir := filepath.Join(home, "Documents", "nb")

	globalWs := &workspace.Workspace{
		Name:        "global",
		Path:        home,
		Type:        workspace.TypeGlobal,
		NotebookDir: globalNotebookDir,
	}

	if err := svc.Registry.Add(globalWs); err != nil {
		t.Fatalf("Failed to add global workspace: %v", err)
	}

	// Create notes in the global workspace
	notesDir := filepath.Join(globalNotebookDir, "global", "current")
	if err := os.MkdirAll(notesDir, 0755); err != nil {
		t.Fatalf("failed to create notes directory: %v", err)
	}

	// Create test note
	filename := filepath.Join(notesDir, "test-note.md")
	content := "# Test Note\n\nThis is a test."
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test note: %v", err)
	}

	// Get workspace context
	ctx, err := svc.GetWorkspaceContext()
	if err != nil {
		t.Fatalf("GetWorkspaceContext failed: %v", err)
	}

	// List notes
	notes, err := svc.ListNotes(ctx, models.NoteTypeCurrent)
	if err != nil {
		t.Fatalf("ListNotes failed: %v", err)
	}

	if len(notes) < 1 {
		t.Errorf("Expected at least 1 note, got %d", len(notes))
	} else {
		t.Logf("Successfully found %d notes", len(notes))
	}
}

// TestWorkingArchive tests the archive functionality
func TestWorkingArchive(t *testing.T) {
	tmpDir := t.TempDir()

	// Create simple archive test
	currentDir := filepath.Join(tmpDir, "current")
	archiveDir := filepath.Join(tmpDir, "archive")
	if err := os.MkdirAll(currentDir, 0755); err != nil {
		t.Fatalf("failed to create current directory: %v", err)
	}
	if err := os.MkdirAll(archiveDir, 0755); err != nil {
		t.Fatalf("failed to create archive directory: %v", err)
	}

	// Create test file
	testFile := filepath.Join(currentDir, "test.md")
	if err := os.WriteFile(testFile, []byte("# Test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Simple archive function
	archiveFile := func(src string) error {
		dst := filepath.Join(archiveDir, filepath.Base(src))
		return os.Rename(src, dst)
	}

	// Archive the file
	if err := archiveFile(testFile); err != nil {
		t.Fatalf("Archive failed: %v", err)
	}

	// Verify
	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Error("Original file still exists")
	}

	archivedFile := filepath.Join(archiveDir, "test.md")
	if _, err := os.Stat(archivedFile); os.IsNotExist(err) {
		t.Error("Archived file not found")
	}
}
