package workspace

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Type represents the type of workspace
type Type string

const (
	TypeGitRepo   Type = "git-repo"
	TypeMonorepo  Type = "monorepo"
	TypeDirectory Type = "directory"
	TypeGlobal    Type = "global"
)

// Workspace represents a registered workspace
type Workspace struct {
	Name        string         `yaml:"name" json:"name"`
	Path        string         `yaml:"path" json:"path"`
	Type        Type           `yaml:"type" json:"type"`
	NotebookDir string         `yaml:"notebook" json:"notebook"`
	Settings    map[string]any `yaml:"settings" json:"settings"`
	CreatedAt   time.Time      `yaml:"created_at" json:"created_at"`
	LastUsed    time.Time      `yaml:"last_used" json:"last_used"`
}

// GetNotePath returns the path for a note in this workspace
func (w *Workspace) GetNotePath(noteType, branch string) string {
	switch w.Type {
	case TypeGitRepo:
		if branch == "" {
			branch = "main"
		}
		return filepath.Join(w.NotebookDir, "repos", w.Name, branch, noteType)
	case TypeGlobal:
		return filepath.Join(w.NotebookDir, "global", noteType)
	default:
		return filepath.Join(w.NotebookDir, w.Name, noteType)
	}
}

// EnsureDirectories creates necessary directories for this workspace
func (w *Workspace) EnsureDirectories(noteType, branch string) error {
	path := w.GetNotePath(noteType, branch)
	return os.MkdirAll(path, 0755)
}

// IsActive checks if we're currently in this workspace
func (w *Workspace) IsActive() bool {
	cwd, err := os.Getwd()
	if err != nil {
		return false
	}

	absPath, err := filepath.Abs(w.Path)
	if err != nil {
		return false
	}

	return strings.HasPrefix(cwd, absPath)
}

// Validate checks if the workspace configuration is valid
func (w *Workspace) Validate() error {
	if w.Name == "" {
		return fmt.Errorf("workspace name cannot be empty")
	}
	if w.Path == "" {
		return fmt.Errorf("workspace path cannot be empty")
	}
	if w.NotebookDir == "" {
		return fmt.Errorf("notebook directory cannot be empty")
	}

	// Expand home directory
	if strings.HasPrefix(w.Path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		w.Path = filepath.Join(home, w.Path[1:])
	}

	if strings.HasPrefix(w.NotebookDir, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		w.NotebookDir = filepath.Join(home, w.NotebookDir[1:])
	}

	return nil
}

// CurrentBranch returns the current git branch for this workspace
func (w *Workspace) CurrentBranch() string {
	if w.Type != TypeGitRepo {
		return ""
	}

	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = w.Path
	output, err := cmd.Output()
	if err != nil {
		return "main"
	}
	return strings.TrimSpace(string(output))
}
