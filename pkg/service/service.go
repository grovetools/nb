package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mattsolo1/grove-notebook/pkg/models"
	"github.com/mattsolo1/grove-notebook/pkg/search"
	"github.com/mattsolo1/grove-notebook/pkg/workspace"
)

// Service is the core note service
type Service struct {
	Registry *workspace.Registry
	Index    *search.Index
	Config   *Config
}

// Config holds service configuration
type Config struct {
	DataDir     string
	Editor      string
	Templates   map[string]string
	DefaultType models.NoteType
}

// New creates a new note service
func New(config *Config) (*Service, error) {
	registry, err := workspace.NewRegistry(config.DataDir)
	if err != nil {
		return nil, fmt.Errorf("create registry: %w", err)
	}

	index, err := search.NewIndex(filepath.Join(config.DataDir, "index.db"))
	if err != nil {
		return nil, fmt.Errorf("create index: %w", err)
	}

	return &Service{
		Registry: registry,
		Index:    index,
		Config:   config,
	}, nil
}

// CreateNote creates a new note in the specified workspace context
func (s *Service) CreateNote(ctx *WorkspaceContext, noteType models.NoteType, title string, options ...CreateOption) (*models.Note, error) {
	opts := &createOptions{
		openEditor: true,
	}
	for _, opt := range options {
		opt(opts)
	}

	// If the global option is used, override the context with the global workspace
	if opts.useGlobal {
		globalWs, err := s.Registry.Global()
		if err != nil {
			return nil, fmt.Errorf("get global workspace: %w", err)
		}
		// Create a new context for the global workspace
		ctx = &WorkspaceContext{
			Workspace: globalWs,
			Branch:    "", // Global workspace has no branch
			Paths:     make(map[string]string),
		}
	}

	// Ensure directory exists
	if err := ctx.Workspace.EnsureDirectories(string(noteType), ctx.Branch); err != nil {
		return nil, fmt.Errorf("ensure directories: %w", err)
	}

	// Generate filename
	var filename string
	if noteType == models.NoteTypeQuick {
		// For quick notes, use just the time + quick
		filename = time.Now().Format("150405") + "-quick.md"
	} else if noteType == models.NoteTypeDaily {
		// For daily notes, use the date as filename
		filename = time.Now().Format("20060102") + "-daily.md"
		// If no title provided, use the date as title
		if title == "" {
			title = "Daily Note: " + time.Now().Format("2006-01-02")
		}
	} else {
		filename = GenerateFilename(title)
	}
	notePath := filepath.Join(ctx.Workspace.GetNotePath(string(noteType), ctx.Branch), filename)

	// Create note content
	template := s.Config.Templates[string(noteType)]
	content := CreateNoteContent(noteType, title, ctx.Workspace.Name, ctx.Branch, template)

	// Write file
	if err := os.WriteFile(notePath, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("write note: %w", err)
	}

	// Parse the created note
	note, err := ParseNote(notePath)
	if err != nil {
		return nil, fmt.Errorf("parse created note: %w", err)
	}

	// Set metadata
	note.Workspace = ctx.Workspace.Name
	note.Branch = ctx.Branch
	note.Type = noteType

	// Index the note
	if err := s.Index.IndexNote(note); err != nil {
		// Don't fail if indexing fails
		fmt.Fprintf(os.Stderr, "Warning: failed to index note: %v\n", err)
	}

	// Open in editor if requested
	if opts.openEditor && s.Config.Editor != "" {
		if err := s.openInEditor(notePath); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to open editor: %v\n", err)
		}
	}

	return note, nil
}

// SearchNotes searches for notes matching the query
func (s *Service) SearchNotes(ctx *WorkspaceContext, query string, options ...SearchOption) ([]*models.Note, error) {
	opts := &searchOptions{
		limit: 50,
	}
	for _, opt := range options {
		opt(opts)
	}

	// Use the provided context's workspace unless searching all workspaces
	var ws *workspace.Workspace
	if !opts.allWorkspaces {
		ws = ctx.Workspace
	}

	results, err := s.Index.Search(query, &search.Options{
		Workspace: ws,
		Type:      string(opts.noteType),
		Limit:     opts.limit,
	})
	if err != nil {
		return nil, fmt.Errorf("search index: %w", err)
	}

	return results, nil
}

// ListNotes lists notes in the current workspace
func (s *Service) ListNotes(ctx *WorkspaceContext, noteType models.NoteType) ([]*models.Note, error) {
	notePath := ctx.Workspace.GetNotePath(string(noteType), ctx.Branch)

	var notes []*models.Note
	err := filepath.Walk(notePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		if !info.IsDir() && strings.HasSuffix(path, ".md") {
			note, err := ParseNote(path)
			if err == nil {
				note.Workspace = ctx.Workspace.Name
				note.Branch = ctx.Branch
				note.Type = noteType
				notes = append(notes, note)
			}
		}
		return nil
	})

	return notes, err
}

// ListAllNotes lists all notes in the specified workspace context (all directories)
func (s *Service) ListAllNotes(ctx *WorkspaceContext) ([]*models.Note, error) {
	// Get the root path for this workspace/branch
	var rootPath string
	if ctx.Workspace.Type == workspace.TypeGlobal {
		rootPath = filepath.Join(ctx.Workspace.NotebookDir, "global")
	} else {
		rootPath = filepath.Join(ctx.Workspace.NotebookDir, "repos", ctx.Workspace.Name, ctx.Branch)
	}

	var notes []*models.Note
	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Skip archive directory
		if strings.Contains(path, "/archive/") {
			return nil
		}

		if !info.IsDir() && strings.HasSuffix(path, ".md") {
			note, err := ParseNote(path)
			if err == nil {
				note.Workspace = ctx.Workspace.Name
				note.Branch = ctx.Branch

				// Extract note type from path
				relPath, _ := filepath.Rel(rootPath, path)
				parts := strings.Split(filepath.ToSlash(relPath), "/")
				if len(parts) > 1 {
					// Everything except the filename is the type
					note.Type = models.NoteType(strings.Join(parts[:len(parts)-1], "/"))
				} else if len(parts) == 1 {
					// If file is at root, infer type from parent directory
					note.Type = models.NoteTypeQuick // Default
				}

				notes = append(notes, note)
			}
		}
		return nil
	})

	return notes, err
}

// ListAllGlobalNotes lists all notes in the global workspace (all directories)
func (s *Service) ListAllGlobalNotes() ([]*models.Note, error) {
	ws, err := s.Registry.Global()
	if err != nil {
		return nil, fmt.Errorf("get global workspace: %w", err)
	}

	ctx := &WorkspaceContext{
		Workspace: ws,
		Branch:    "",
		Paths:     make(map[string]string),
	}

	return s.ListAllNotes(ctx)
}

// ListGlobalNotes lists notes in the global workspace
func (s *Service) ListGlobalNotes(noteType models.NoteType) ([]*models.Note, error) {
	ws, err := s.Registry.Global()
	if err != nil {
		return nil, fmt.Errorf("get global workspace: %w", err)
	}

	ctx := &WorkspaceContext{
		Workspace: ws,
		Branch:    "",
		Paths:     make(map[string]string),
	}

	return s.ListNotes(ctx, noteType)
}

// ArchiveNotes moves notes to the archive
func (s *Service) ArchiveNotes(ctx *WorkspaceContext, paths []string) error {
	archivePath := ctx.Workspace.GetNotePath("archive", ctx.Branch)
	if err := os.MkdirAll(archivePath, 0755); err != nil {
		return fmt.Errorf("create archive directory: %w", err)
	}

	for _, path := range paths {
		// Get relative path within note directory
		_, _, noteType := GetNoteMetadata(path)
		if noteType == "" {
			continue
		}

		// Create subdirectory in archive if needed
		archiveSubdir := filepath.Join(archivePath, noteType)
		if err := os.MkdirAll(archiveSubdir, 0755); err != nil {
			return fmt.Errorf("create archive subdirectory: %w", err)
		}

		// Move file
		dest := filepath.Join(archiveSubdir, filepath.Base(path))
		if err := os.Rename(path, dest); err != nil {
			return fmt.Errorf("move %s to archive: %w", path, err)
		}

		// Update index
		if note, err := ParseNote(dest); err == nil {
			note.IsArchived = true
			note.Workspace = ctx.Workspace.Name
			note.Branch = ctx.Branch
			if err := s.Index.IndexNote(note); err != nil {
				// Don't fail the operation, just log the error
				fmt.Fprintf(os.Stderr, "Warning: failed to index archived note: %v\n", err)
			}
		}
	}

	return nil
}

// GetWorkspaceContext returns current workspace context
func (s *Service) GetWorkspaceContext() (*WorkspaceContext, error) {
	ws, err := s.Registry.DetectCurrent()
	if err != nil {
		return nil, err
	}

	branch := ""
	if ws.Type == workspace.TypeGitRepo {
		branch = getCurrentGitBranch(ws.Path)
	}

	return &WorkspaceContext{
		Workspace: ws,
		Branch:    branch,
		Paths: map[string]string{
			"current":      ws.GetNotePath("current", branch),
			"llm":          ws.GetNotePath("llm", branch),
			"learn":        ws.GetNotePath("learn", branch),
			"daily":        ws.GetNotePath("daily", branch),
			"issues":       ws.GetNotePath("issues", branch),
			"architecture": ws.GetNotePath("architecture", branch),
			"todos":        ws.GetNotePath("todos", branch),
			"quick":        ws.GetNotePath("quick", branch),
			"archive":      ws.GetNotePath("archive", branch),
			"prompts":      ws.GetNotePath("prompts", branch),
		},
	}, nil
}

// openInEditor opens a file in the configured editor
func (s *Service) openInEditor(path string) error {
	editor := s.Config.Editor
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vim" // fallback
	}

	cmd := exec.Command(editor, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// UpdateNoteContent updates the content of an existing note
func (s *Service) UpdateNoteContent(path string, content string) error {
	// Write the new content
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("write note: %w", err)
	}

	// Re-index the note
	note, err := ParseNote(path)
	if err != nil {
		return fmt.Errorf("parse note: %w", err)
	}

	// Extract workspace and branch from path
	ws, branch, noteType := GetNoteMetadata(path)
	note.Workspace = ws
	note.Branch = branch
	note.Type = models.NoteType(noteType)

	// Index the updated note
	if err := s.Index.IndexNote(note); err != nil {
		// Don't fail if indexing fails
		fmt.Fprintf(os.Stderr, "Warning: failed to index note: %v\n", err)
	}

	return nil
}

// BuildNotePath constructs a path for a note in the specified workspace/branch/type
func (s *Service) BuildNotePath(workspaceName, branch, noteType, filename string) (string, error) {
	var ws *workspace.Workspace

	if workspaceName == "global" || workspaceName == "" {
		// Use global workspace
		globalWs, err := s.Registry.Global()
		if err != nil {
			return "", fmt.Errorf("get global workspace: %w", err)
		}
		ws = globalWs
	} else {
		// Find workspace by name
		workspaces, err := s.Registry.List()
		if err != nil {
			return "", fmt.Errorf("list workspaces: %w", err)
		}

		for _, w := range workspaces {
			if w.Name == workspaceName {
				ws = w
				break
			}
		}

		if ws == nil {
			return "", fmt.Errorf("workspace not found: %s", workspaceName)
		}
	}

	// Get the base path for the note type
	notePath := ws.GetNotePath(noteType, branch)

	// Combine with filename
	return filepath.Join(notePath, filename), nil
}

// IndexFile indexes a single file
func (s *Service) IndexFile(path string) error {
	note, err := ParseNote(path)
	if err != nil {
		return fmt.Errorf("parse note: %w", err)
	}

	// Extract metadata from path
	ws, branch, noteType := GetNoteMetadata(path)
	note.Workspace = ws
	note.Branch = branch
	note.Type = models.NoteType(noteType)

	return s.Index.IndexNote(note)
}

// Close closes the service
func (s *Service) Close() error {
	if err := s.Index.Close(); err != nil {
		return err
	}
	return s.Registry.Close()
}

// WorkspaceContext holds current workspace information
type WorkspaceContext struct {
	Workspace *workspace.Workspace
	Branch    string
	Paths     map[string]string
}

// Options for various operations
type createOptions struct {
	openEditor bool
	useGlobal  bool
}

type CreateOption func(*createOptions)

func WithoutEditor() CreateOption {
	return func(o *createOptions) {
		o.openEditor = false
	}
}

func InGlobalWorkspace() CreateOption {
	return func(o *createOptions) {
		o.useGlobal = true
	}
}

type searchOptions struct {
	workspace     *workspace.Workspace
	allWorkspaces bool
	noteType      models.NoteType
	limit         int
}

type SearchOption func(*searchOptions)

func InWorkspace(w *workspace.Workspace) SearchOption {
	return func(o *searchOptions) {
		o.workspace = w
	}
}

func AllWorkspaces() SearchOption {
	return func(o *searchOptions) {
		o.allWorkspaces = true
	}
}

func OfType(t models.NoteType) SearchOption {
	return func(o *searchOptions) {
		o.noteType = t
	}
}

func WithLimit(limit int) SearchOption {
	return func(o *searchOptions) {
		o.limit = limit
	}
}

// getCurrentGitBranch returns the current git branch
func getCurrentGitBranch(repoPath string) string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return "main"
	}
	return strings.TrimSpace(string(output))
}

// ListAllNotesInWorkspace lists all notes in a given workspace, across all branches.
func (s *Service) ListAllNotesInWorkspace(ws *workspace.Workspace) ([]*models.Note, error) {
	if ws.Type != workspace.TypeGitRepo {
		// This functionality is most relevant for git repos.
		// For other types, it might behave like ListAllNotes.
		// For now, let's focus on the git repo case.
		return nil, fmt.Errorf("listing notes across all branches is only supported for git-repo workspaces")
	}

	// The root for a repository's notes is at .../repos/<workspace-name>/
	repoNotesRoot := filepath.Join(ws.NotebookDir, "repos", ws.Name)

	var notes []*models.Note
	err := filepath.Walk(repoNotesRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors to continue walking
		}

		// Skip archive directories within any branch
		if strings.Contains(path, "/archive/") {
			return filepath.SkipDir
		}

		if !info.IsDir() && strings.HasSuffix(path, ".md") {
			note, err := ParseNote(path)
			if err == nil {
				// ParseNote already extracts workspace, branch, and type from the path
				notes = append(notes, note)
			}
		}
		return nil
	})

	return notes, err
}

// GetBranches returns all branches for a git workspace
func (s *Service) GetBranches(ws *workspace.Workspace) ([]string, error) {
	if ws.Type != workspace.TypeGitRepo {
		return []string{}, nil
	}

	// Get local branches
	cmd := exec.Command("git", "branch", "--format=%(refname:short)")
	cmd.Dir = ws.Path
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("list branches: %w", err)
	}

	// Parse branches from output
	branches := []string{}
	for _, line := range strings.Split(string(output), "\n") {
		branch := strings.TrimSpace(line)
		if branch != "" {
			branches = append(branches, branch)
		}
	}

	// If no branches found, return main as default
	if len(branches) == 0 {
		branches = []string{"main"}
	}

	return branches, nil
}
