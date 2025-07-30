package workspace

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWorkspaceGetNotePath(t *testing.T) {
	tests := []struct {
		name     string
		ws       *Workspace
		noteType string
		branch   string
		want     string
	}{
		{
			name: "Git repo workspace with branch",
			ws: &Workspace{
				Name:        "test-repo",
				Type:        TypeGitRepo,
				NotebookDir: "/home/user/nb",
			},
			noteType: "current",
			branch:   "feature/test",
			want:     "/home/user/nb/repos/test-repo/feature/test/current",
		},
		{
			name: "Git repo workspace without branch",
			ws: &Workspace{
				Name:        "test-repo",
				Type:        TypeGitRepo,
				NotebookDir: "/home/user/nb",
			},
			noteType: "current",
			branch:   "",
			want:     "/home/user/nb/repos/test-repo/main/current",
		},
		{
			name: "Global workspace",
			ws: &Workspace{
				Name:        "global",
				Type:        TypeGlobal,
				NotebookDir: "/home/user/nb",
			},
			noteType: "quick",
			branch:   "",
			want:     "/home/user/nb/global/quick",
		},
		{
			name: "Directory workspace",
			ws: &Workspace{
				Name:        "project",
				Type:        TypeDirectory,
				NotebookDir: "/home/user/nb",
			},
			noteType: "todos",
			branch:   "",
			want:     "/home/user/nb/project/todos",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.ws.GetNotePath(tt.noteType, tt.branch)
			if got != tt.want {
				t.Errorf("GetNotePath() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWorkspaceEnsureDirectories(t *testing.T) {
	tmpDir := t.TempDir()

	ws := &Workspace{
		Name:        "test-workspace",
		Type:        TypeDirectory,
		NotebookDir: tmpDir,
	}

	// Test creating directories
	err := ws.EnsureDirectories("current", "")
	if err != nil {
		t.Fatalf("EnsureDirectories failed: %v", err)
	}

	// Check if directory was created
	expectedPath := filepath.Join(tmpDir, "test-workspace", "current")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("Expected directory %s to exist", expectedPath)
	}
}

func TestDetectWorkspaceType(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func(string) error
		wantType  Type
	}{
		{
			name: "Git repository",
			setupFunc: func(dir string) error {
				return os.MkdirAll(filepath.Join(dir, ".git"), 0755)
			},
			wantType: TypeGitRepo,
		},
		{
			name: "Regular directory",
			setupFunc: func(dir string) error {
				return nil
			},
			wantType: TypeDirectory,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			if err := tt.setupFunc(tmpDir); err != nil {
				t.Fatalf("Setup failed: %v", err)
			}

			wsType := detectType(tmpDir)
			if wsType != tt.wantType {
				t.Errorf("Expected workspace type %v, got %v", tt.wantType, wsType)
			}
		})
	}
}

func TestWorkspaceTimeFields(t *testing.T) {
	now := time.Now()

	ws := &Workspace{
		Name:        "test",
		Path:        "/test",
		Type:        TypeDirectory,
		NotebookDir: "/test/nb",
		CreatedAt:   now,
		LastUsed:    now.Add(24 * time.Hour),
	}

	// Test that time fields are properly set
	if ws.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}

	if ws.LastUsed.Before(ws.CreatedAt) {
		t.Error("LastUsed should be after CreatedAt")
	}
}

func TestWorkspaceSettings(t *testing.T) {
	ws := &Workspace{
		Name: "test",
		Settings: map[string]any{
			"editor":      "nvim",
			"auto_commit": true,
			"template":    "default",
		},
	}

	// Test settings access
	if editor, ok := ws.Settings["editor"].(string); !ok || editor != "nvim" {
		t.Error("Expected editor setting to be 'nvim'")
	}

	if autoCommit, ok := ws.Settings["auto_commit"].(bool); !ok || !autoCommit {
		t.Error("Expected auto_commit setting to be true")
	}
}

// Helper function to detect workspace type (simplified version)
func detectType(path string) Type {
	if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
		return TypeGitRepo
	}
	return TypeDirectory
}
