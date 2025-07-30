package workspace

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewRegistry(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data")

	reg, err := NewRegistry(dataDir)
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}
	defer reg.Close()

	if reg.dataDir != dataDir {
		t.Errorf("Expected dataDir %s, got %s", dataDir, reg.dataDir)
	}

	// Check if database file was created
	dbFile := filepath.Join(dataDir, "workspaces.db")
	if _, err := os.Stat(dbFile); os.IsNotExist(err) {
		t.Error("Expected database file to be created")
	}
}

func TestAddAndGetWorkspace(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data")

	reg, err := NewRegistry(dataDir)
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}
	defer reg.Close()

	// Create test workspace
	ws := &Workspace{
		Name:        "test-workspace",
		Path:        filepath.Join(tmpDir, "test-workspace"),
		Type:        TypeDirectory,
		NotebookDir: filepath.Join(tmpDir, "nb"),
		Settings:    map[string]any{"editor": "vim"},
		CreatedAt:   time.Now(),
		LastUsed:    time.Now(),
	}

	// Add workspace
	if err := reg.Add(ws); err != nil {
		t.Fatalf("Failed to add workspace: %v", err)
	}

	// Get workspace
	retrieved, err := reg.Get("test-workspace")
	if err != nil {
		t.Fatalf("Failed to get workspace: %v", err)
	}

	if retrieved.Name != ws.Name {
		t.Errorf("Expected workspace name %s, got %s", ws.Name, retrieved.Name)
	}

	if retrieved.Type != ws.Type {
		t.Errorf("Expected workspace type %s, got %s", ws.Type, retrieved.Type)
	}

	// Test Get non-existent
	_, err = reg.Get("non-existent")
	if err == nil {
		t.Error("Expected error when getting non-existent workspace")
	}
}

func TestListWorkspaces(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data")

	reg, err := NewRegistry(dataDir)
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}
	defer reg.Close()

	// Add multiple workspaces
	workspaces := []*Workspace{
		{
			Name:        "workspace1",
			Path:        filepath.Join(tmpDir, "ws1"),
			Type:        TypeDirectory,
			NotebookDir: filepath.Join(tmpDir, "nb"),
		},
		{
			Name:        "workspace2",
			Path:        filepath.Join(tmpDir, "ws2"),
			Type:        TypeGitRepo,
			NotebookDir: filepath.Join(tmpDir, "nb"),
		},
	}

	for _, ws := range workspaces {
		if err := reg.Add(ws); err != nil {
			t.Fatalf("Failed to add workspace: %v", err)
		}
	}

	// List workspaces
	listed, err := reg.List()
	if err != nil {
		t.Fatalf("Failed to list workspaces: %v", err)
	}

	// Should have at least the workspaces we added
	if len(listed) < len(workspaces) {
		t.Errorf("Expected at least %d workspaces, got %d", len(workspaces), len(listed))
	}
}

func TestRemoveWorkspace(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data")

	reg, err := NewRegistry(dataDir)
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}
	defer reg.Close()

	// Add workspace
	ws := &Workspace{
		Name:        "test-workspace",
		Path:        filepath.Join(tmpDir, "test-workspace"),
		Type:        TypeDirectory,
		NotebookDir: filepath.Join(tmpDir, "nb"),
	}

	if err := reg.Add(ws); err != nil {
		t.Fatalf("Failed to add workspace: %v", err)
	}

	// Remove workspace
	if err := reg.Remove("test-workspace"); err != nil {
		t.Fatalf("Failed to remove workspace: %v", err)
	}

	// Should not be able to get removed workspace
	_, err = reg.Get("test-workspace")
	if err == nil {
		t.Error("Expected error when getting removed workspace")
	}
}

func TestFindByPath(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data")

	reg, err := NewRegistry(dataDir)
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}
	defer reg.Close()

	// Create nested workspace structure
	parentDir := filepath.Join(tmpDir, "parent")
	childDir := filepath.Join(parentDir, "child")
	if err := os.MkdirAll(childDir, 0755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	// Add workspace
	ws := &Workspace{
		Name:        "parent-workspace",
		Path:        parentDir,
		Type:        TypeDirectory,
		NotebookDir: filepath.Join(tmpDir, "nb"),
	}

	if err := reg.Add(ws); err != nil {
		t.Fatalf("Failed to add workspace: %v", err)
	}

	// Test finding workspace from child directory
	found, err := reg.FindByPath(childDir)
	if err != nil {
		t.Fatalf("Failed to find workspace by path: %v", err)
	}

	if found.Name != "parent-workspace" {
		t.Errorf("Expected to find parent-workspace, got %s", found.Name)
	}

	// Test finding from exact path
	found, err = reg.FindByPath(parentDir)
	if err != nil {
		t.Fatalf("Failed to find workspace by exact path: %v", err)
	}

	if found.Name != "parent-workspace" {
		t.Errorf("Expected to find parent-workspace, got %s", found.Name)
	}

	// Test with non-workspace path
	found, err = reg.FindByPath(tmpDir)
	if err != nil {
		t.Fatalf("FindByPath returned error: %v", err)
	}
	if found != nil {
		t.Error("Expected nil workspace for non-workspace path")
	}
}
