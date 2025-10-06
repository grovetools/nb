package workspace

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Registry manages workspace registration and detection
type Registry struct {
	db              *sql.DB
	dataDir         string
	globalWorkspace *Workspace
}

// NewRegistry creates a new workspace registry
func NewRegistry(dataDir string) (*Registry, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	dbPath := filepath.Join(dataDir, "workspaces.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	r := &Registry{
		db:      db,
		dataDir: dataDir,
	}

	if err := r.init(); err != nil {
		return nil, fmt.Errorf("initialize registry: %w", err)
	}

	return r, nil
}

// init creates the database schema
func (r *Registry) init() error {
	schema := `
	CREATE TABLE IF NOT EXISTS workspaces (
		name TEXT PRIMARY KEY,
		path TEXT NOT NULL,
		type TEXT NOT NULL,
		notebook_dir TEXT NOT NULL,
		settings TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		last_used TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_workspaces_path ON workspaces(path);
	`

	_, err := r.db.Exec(schema)
	return err
}

// Add registers a new workspace
func (r *Registry) Add(w *Workspace) error {
	if err := w.Validate(); err != nil {
		return fmt.Errorf("validate workspace: %w", err)
	}

	settings, err := json.Marshal(w.Settings)
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}

	query := `
	INSERT OR REPLACE INTO workspaces (name, path, type, notebook_dir, settings, created_at, last_used)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	now := time.Now()
	_, err = r.db.Exec(query, w.Name, w.Path, w.Type, w.NotebookDir, settings, now, now)
	return err
}

// Get retrieves a workspace by name
func (r *Registry) Get(name string) (*Workspace, error) {
	query := `
	SELECT name, path, type, notebook_dir, settings, created_at, last_used
	FROM workspaces WHERE name = ?
	`

	w := &Workspace{}
	var settings string
	err := r.db.QueryRow(query, name).Scan(
		&w.Name, &w.Path, &w.Type, &w.NotebookDir,
		&settings, &w.CreatedAt, &w.LastUsed,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("workspace not found: %s", name)
	}
	if err != nil {
		return nil, err
	}

	if settings != "" {
		if err := json.Unmarshal([]byte(settings), &w.Settings); err != nil {
			return nil, fmt.Errorf("unmarshal settings: %w", err)
		}
	}

	return w, nil
}

// List returns all registered workspaces
func (r *Registry) List() ([]*Workspace, error) {
	query := `
	SELECT name, path, type, notebook_dir, settings, created_at, last_used
	FROM workspaces ORDER BY last_used DESC
	`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workspaces []*Workspace
	for rows.Next() {
		w := &Workspace{}
		var settings string
		err := rows.Scan(
			&w.Name, &w.Path, &w.Type, &w.NotebookDir,
			&settings, &w.CreatedAt, &w.LastUsed,
		)
		if err != nil {
			return nil, err
		}

		if settings != "" {
			if err := json.Unmarshal([]byte(settings), &w.Settings); err != nil {
				return nil, fmt.Errorf("unmarshal settings: %w", err)
			}
		}

		workspaces = append(workspaces, w)
	}

	return workspaces, nil
}

// FindByPath finds a workspace that contains the given path
func (r *Registry) FindByPath(path string) (*Workspace, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	workspaces, err := r.List()
	if err != nil {
		return nil, err
	}

	// Find the most specific workspace match
	var bestMatch *Workspace
	bestMatchLen := 0

	// Convert to lowercase for case-insensitive comparison
	lowerAbsPath := strings.ToLower(absPath)

	for _, w := range workspaces {
		wsAbsPath, err := filepath.Abs(w.Path)
		if err != nil {
			continue
		}

		// Case-insensitive comparison to handle filesystem case variations
		lowerWsAbsPath := strings.ToLower(wsAbsPath)
		if strings.HasPrefix(lowerAbsPath, lowerWsAbsPath) && len(wsAbsPath) > bestMatchLen {
			bestMatch = w
			bestMatchLen = len(wsAbsPath)
		}
	}

	if bestMatch != nil {
		// Update last used time
		if err := r.updateLastUsed(bestMatch.Name); err != nil {
			// Log error but don't fail the detection
			fmt.Fprintf(os.Stderr, "Warning: failed to update last used: %v\n", err)
		}
		return bestMatch, nil
	}

	return nil, nil
}

// DetectCurrent detects the workspace for the current directory
func (r *Registry) DetectCurrent() (*Workspace, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	// First check registered workspaces
	if w, err := r.FindByPath(cwd); err == nil && w != nil {
		return w, nil
	}

	// Check if we're in a git repo and auto-register
	if gitRoot := findGitRoot(cwd); gitRoot != "" {
		return r.AutoRegister(gitRoot)
	}

	// Fall back to global workspace
	return r.Global()
}

// AutoRegister automatically registers a git repository as a workspace
func (r *Registry) AutoRegister(path string) (*Workspace, error) {
	name := filepath.Base(path)

	// Check if already registered
	if w, err := r.Get(name); err == nil {
		return w, nil
	}

	// Get default notebook directory from config
	notebookDir := GetDefaultNotebookDir()

	w := &Workspace{
		Name:        name,
		Path:        path,
		Type:        TypeGitRepo,
		NotebookDir: notebookDir,
		Settings: map[string]any{
			"auto_registered":  true,
			"auto_branch_dirs": true,
		},
	}

	if err := r.Add(w); err != nil {
		return nil, fmt.Errorf("auto-register workspace: %w", err)
	}

	return w, nil
}

// Global returns the global workspace
func (r *Registry) Global() (*Workspace, error) {
	if r.globalWorkspace != nil {
		return r.globalWorkspace, nil
	}

	// Try to get from registry
	if w, err := r.Get("global"); err == nil {
		r.globalWorkspace = w
		return w, nil
	}

	// Create default global workspace
	home, _ := os.UserHomeDir()
	w := &Workspace{
		Name:        "global",
		Path:        home,
		Type:        TypeGlobal,
		NotebookDir: GetDefaultNotebookDir(),
		Settings:    map[string]any{},
	}

	if err := r.Add(w); err != nil {
		return nil, fmt.Errorf("create global workspace: %w", err)
	}

	r.globalWorkspace = w
	return w, nil
}

// Remove removes a workspace from the registry
func (r *Registry) Remove(name string) error {
	_, err := r.db.Exec("DELETE FROM workspaces WHERE name = ?", name)
	return err
}

// updateLastUsed updates the last used timestamp for a workspace
func (r *Registry) updateLastUsed(name string) error {
	_, err := r.db.Exec(
		"UPDATE workspaces SET last_used = ? WHERE name = ?",
		time.Now(), name,
	)
	return err
}

// Close closes the registry database
func (r *Registry) Close() error {
	return r.db.Close()
}

// findGitRoot finds the root of a git repository
func findGitRoot(path string) string {
	current := path
	for {
		if _, err := os.Stat(filepath.Join(current, ".git")); err == nil {
			return current
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return ""
}

