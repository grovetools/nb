// +build integration

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mattsolo1/grove-notebook/pkg/models"
	"github.com/mattsolo1/grove-notebook/pkg/service"
	"github.com/mattsolo1/grove-notebook/pkg/workspace"
)

func TestIntegration(t *testing.T) {
	// Skip if not running integration tests
	if os.Getenv("RUN_INTEGRATION_TESTS") == "" {
		t.Skip("Skipping integration test. Set RUN_INTEGRATION_TESTS=1 to run.")
	}

	tmpDir := t.TempDir()

	// Test 1: Create service
	t.Run("CreateService", func(t *testing.T) {
		config := &service.Config{
			DataDir:     filepath.Join(tmpDir, "data"),
			Editor:      "vim",
			DefaultType: models.NoteTypeCurrent,
		}

		svc, err := service.New(config)
		if err != nil {
			t.Fatalf("Failed to create service: %v", err)
		}
		defer svc.Close()

		if svc.Config == nil {
			t.Error("Service config is nil")
		}
	})

	// Test 2: Registry operations
	t.Run("RegistryOperations", func(t *testing.T) {
		reg, err := workspace.NewRegistry(filepath.Join(tmpDir, "registry"))
		if err != nil {
			t.Fatalf("Failed to create registry: %v", err)
		}
		defer reg.Close()

		// Add workspace
		ws := &workspace.Workspace{
			Name:        "test",
			Path:        filepath.Join(tmpDir, "workspace"),
			Type:        workspace.TypeDirectory,
			NotebookDir: filepath.Join(tmpDir, "notebook"),
		}

		if err := reg.Add(ws); err != nil {
			t.Fatalf("Failed to add workspace: %v", err)
		}

		// Get workspace
		retrieved, err := reg.Get("test")
		if err != nil {
			t.Fatalf("Failed to get workspace: %v", err)
		}

		if retrieved.Name != "test" {
			t.Errorf("Expected workspace name 'test', got %s", retrieved.Name)
		}
	})

	// Test 3: Note types
	t.Run("NoteTypes", func(t *testing.T) {
		validTypes := []models.NoteType{
			models.NoteTypeCurrent,
			models.NoteTypeQuick,
			models.NoteTypeLLM,
			models.NoteTypeLearn,
			models.NoteTypeDaily,
			models.NoteTypeIssues,
			models.NoteTypeArchitecture,
			models.NoteTypeTodos,
		}

		for _, nt := range validTypes {
			if nt == "" {
				t.Errorf("Note type should not be empty: %v", nt)
			}
		}
	})
}

func TestEndToEnd(t *testing.T) {
	if os.Getenv("RUN_E2E_TESTS") == "" {
		t.Skip("Skipping E2E test. Set RUN_E2E_TESTS=1 to run.")
	}

	tmpDir := t.TempDir()

	// Setup
	config := &service.Config{
		DataDir:     filepath.Join(tmpDir, "data"),
		Editor:      "vim",
		DefaultType: models.NoteTypeCurrent,
	}

	svc, err := service.New(config)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}
	defer svc.Close()

	// Create workspace
	wsPath := filepath.Join(tmpDir, "my-project")
	os.MkdirAll(wsPath, 0755)
	os.MkdirAll(filepath.Join(wsPath, ".git"), 0755)

	// Register workspace
	ws := &workspace.Workspace{
		Name:        "my-project",
		Path:        wsPath,
		Type:        workspace.TypeGitRepo,
		NotebookDir: filepath.Join(tmpDir, "notebooks"),
	}

	if err := svc.Registry.Add(ws); err != nil {
		t.Fatalf("Failed to register workspace: %v", err)
	}

	// Verify registration
	registered, err := svc.Registry.Get("my-project")
	if err != nil {
		t.Fatalf("Failed to get registered workspace: %v", err)
	}

	if registered.Name != "my-project" {
		t.Errorf("Expected workspace name 'my-project', got %s", registered.Name)
	}

	t.Logf("Successfully completed end-to-end test")
}