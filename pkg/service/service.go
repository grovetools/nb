package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	coreconfig "github.com/mattsolo1/grove-core/config"
	"github.com/mattsolo1/grove-core/git"
	coreworkspace "github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-notebook/pkg/models"
	"github.com/mattsolo1/grove-notebook/pkg/search"
	"github.com/mattsolo1/grove-notebook/pkg/workspace"
)

// Service is the core note service
type Service struct {
	workspaceProvider *coreworkspace.Provider
	notebookLocator   *coreworkspace.NotebookLocator
	Index             *search.Index
	Config            *Config
}

// Config holds service configuration
type Config struct {
	DataDir     string
	Editor      string
	Templates   map[string]string
	DefaultType models.NoteType
}

// New creates a new note service
func New(config *Config, provider *coreworkspace.Provider) (*Service, error) {
	index, err := search.NewIndex(filepath.Join(config.DataDir, "index.db"))
	if err != nil {
		return nil, fmt.Errorf("create index: %w", err)
	}

	// Load core config and initialize NotebookLocator
	coreCfg, err := coreconfig.LoadDefault()
	if err != nil {
		// Proceed with empty config if none exists (Local Mode)
		coreCfg = &coreconfig.Config{}
	}

	// Check for deprecated nb.notebook_dir configuration
	var nbConfig workspace.NbConfig
	if err := coreCfg.UnmarshalExtension("nb", &nbConfig); err == nil && nbConfig.NotebookDir != "" {
		fmt.Fprintln(os.Stderr, "⚠️  Warning: The 'nb.notebook_dir' config is deprecated. Please configure 'notebook.root_dir' in your global grove.yml instead.")
	}

	notebookLocator := coreworkspace.NewNotebookLocator(coreCfg)

	return &Service{
		workspaceProvider: provider,
		notebookLocator:   notebookLocator,
		Index:             index,
		Config:            config,
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

	var currentContext *WorkspaceContext
	var err error

	if opts.useGlobal {
		// Create a specific context for the global workspace
		currentContext, err = s.GetWorkspaceContext("global")
	} else {
		currentContext = ctx
	}
	if err != nil {
		return nil, fmt.Errorf("get workspace context for create: %w", err)
	}

	// Ensure directory exists
	noteDir, err := s.getNotePathForContext(currentContext, string(noteType))
	if err != nil {
		return nil, fmt.Errorf("get note path: %w", err)
	}
	if err := os.MkdirAll(noteDir, 0755); err != nil {
		return nil, fmt.Errorf("ensure directories: %w", err)
	}

	// Generate filename
	var filename string
	if noteType == models.NoteTypeQuick {
		filename = time.Now().Format("150405") + "-quick.md"
	} else if noteType == models.NoteTypeDaily {
		filename = time.Now().Format("20060102") + "-daily.md"
		if title == "" {
			title = "Daily Note: " + time.Now().Format("2006-01-02")
		}
	} else {
		filename = GenerateFilename(title)
	}
	notePath := filepath.Join(noteDir, filename)

	// Create note content
	template := s.Config.Templates[string(noteType)]
	content := CreateNoteContent(noteType, title, currentContext.NotebookContextWorkspace.Name, currentContext.Branch, currentContext.CurrentWorkspace.Name, template)

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
	note.Workspace = currentContext.NotebookContextWorkspace.Name
	note.Branch = currentContext.Branch
	note.Type = noteType

	// Index the note
	if err := s.Index.IndexNote(note); err != nil {
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

	var searchWorkspaceName string
	if !opts.allWorkspaces {
		searchWorkspaceName = ctx.NotebookContextWorkspace.Name
	}

	results, err := s.Index.Search(query, &search.Options{
		WorkspaceName: searchWorkspaceName,
		Type:          string(opts.noteType),
		Limit:         opts.limit,
	})
	if err != nil {
		return nil, fmt.Errorf("search index: %w", err)
	}

	return results, nil
}

// ListNotes lists notes in the current workspace
func (s *Service) ListNotes(ctx *WorkspaceContext, noteType models.NoteType) ([]*models.Note, error) {
	notePath, err := s.getNotePathForContext(ctx, string(noteType))
	if err != nil {
		return nil, fmt.Errorf("get note path: %w", err)
	}

	var notes []*models.Note
	err = filepath.Walk(notePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		if !info.IsDir() && strings.HasSuffix(path, ".md") {
			note, err := ParseNote(path)
			if err == nil {
				note.Workspace = ctx.NotebookContextWorkspace.Name
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
	// Get the root path by getting any note type directory and going up one level
	// This works because all note types share the same parent directory
	samplePath, err := s.notebookLocator.GetNotesDir(ctx.NotebookContextWorkspace, "current")
	if err != nil {
		return nil, fmt.Errorf("get notes root: %w", err)
	}
	// Go up one level to get the parent directory that contains all note types
	rootPath := filepath.Dir(samplePath)

	var notes []*models.Note
	err = filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if strings.Contains(path, "/archive/") {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(path, ".md") {
			note, err := ParseNote(path)
			if err == nil {
				note.Workspace = ctx.NotebookContextWorkspace.Name
				note.Branch = ctx.Branch

				relPath, _ := filepath.Rel(rootPath, path)
				parts := strings.Split(filepath.ToSlash(relPath), "/")
				if len(parts) > 1 {
					note.Type = models.NoteType(strings.Join(parts[:len(parts)-1], "/"))
				} else if len(parts) == 1 {
					note.Type = models.NoteTypeQuick
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
	ctx, err := s.GetWorkspaceContext("global")
	if err != nil {
		return nil, fmt.Errorf("get global workspace context: %w", err)
	}
	return s.ListAllNotes(ctx)
}

// ListGlobalNotes lists notes in the global workspace
func (s *Service) ListGlobalNotes(noteType models.NoteType) ([]*models.Note, error) {
	ctx, err := s.GetWorkspaceContext("global")
	if err != nil {
		return nil, fmt.Errorf("get global workspace context: %w", err)
	}
	return s.ListNotes(ctx, noteType)
}

// ArchiveNotes moves notes to the archive
func (s *Service) ArchiveNotes(ctx *WorkspaceContext, paths []string) error {
	archivePath, err := s.getNotePathForContext(ctx, "archive")
	if err != nil {
		return fmt.Errorf("get archive path: %w", err)
	}
	if err := os.MkdirAll(archivePath, 0755); err != nil {
		return fmt.Errorf("create archive directory: %w", err)
	}

	for _, path := range paths {
		_, _, noteType := GetNoteMetadata(path)
		if noteType == "" {
			continue
		}

		archiveSubdir := filepath.Join(archivePath, noteType)
		if err := os.MkdirAll(archiveSubdir, 0755); err != nil {
			return fmt.Errorf("create archive subdirectory: %w", err)
		}

		dest := filepath.Join(archiveSubdir, filepath.Base(path))
		if err := os.Rename(path, dest); err != nil {
			return fmt.Errorf("move %s to archive: %w", path, err)
		}

		if note, err := ParseNote(dest); err == nil {
			note.IsArchived = true
			note.Workspace = ctx.NotebookContextWorkspace.Name
			note.Branch = ctx.Branch
			if err := s.Index.IndexNote(note); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to index archived note: %v\n", err)
			}
		}
	}
	return nil
}

// GetWorkspaceContext returns current workspace context.
// If startPath is provided, it's used as the basis for context detection.
// If startPath is "global", it forces the global context.
func (s *Service) GetWorkspaceContext(startPath string) (*WorkspaceContext, error) {
	if startPath == "global" {
		// For global context, use a minimal WorkspaceNode with just the name
		globalNode := &coreworkspace.WorkspaceNode{Name: "global", Path: ""}
		paths, err := s.buildPathsMap(globalNode)
		if err != nil {
			return nil, fmt.Errorf("build paths map for global: %w", err)
		}
		return &WorkspaceContext{
			CurrentWorkspace:         globalNode,
			NotebookContextWorkspace: globalNode,
			Branch:                   "",
			Paths:                    paths,
		}, nil
	}

	var CWD string
	var err error
	if startPath == "" {
		CWD, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	} else {
		CWD = startPath
	}

	currentWorkspace, err := coreworkspace.GetProjectByPath(CWD)
	if err != nil {
		// Fallback to global context if not in a known workspace
		return s.GetWorkspaceContext("global")
	}

	notebookContextWorkspace, err := s.findNotebookContextNode(currentWorkspace)
	if err != nil {
		return nil, fmt.Errorf("could not determine notebook context for '%s': %w", currentWorkspace.Name, err)
	}

	branch := ""
	if git.IsGitRepo(currentWorkspace.Path) {
		_, branch, _ = git.GetRepoInfo(currentWorkspace.Path)
	}

	paths, err := s.buildPathsMap(notebookContextWorkspace)
	if err != nil {
		return nil, fmt.Errorf("build paths map: %w", err)
	}

	return &WorkspaceContext{
		CurrentWorkspace:         currentWorkspace,
		NotebookContextWorkspace: notebookContextWorkspace,
		Branch:                   branch,
		Paths:                    paths,
	}, nil
}

// findNotebookContextNode determines the logical owner of the notebook directory.
// The notebook context is always the repository (project) that manages the git history.
// For ecosystem worktree subprojects, this means finding the corresponding subproject in the main ecosystem.
func (s *Service) findNotebookContextNode(currentNode *coreworkspace.WorkspaceNode) (*coreworkspace.WorkspaceNode, error) {
	switch currentNode.Kind {
	case coreworkspace.KindEcosystemWorktreeSubProjectWorktree:
		// For a subproject within an ecosystem worktree (e.g., grove-notebook inside general-refactoring),
		// we want to use the corresponding subproject in the main ecosystem (e.g., grove-notebook in grove-ecosystem)
		// Find the main ecosystem root
		if currentNode.RootEcosystemPath == "" {
			return nil, fmt.Errorf("ecosystem worktree subproject has no root ecosystem path")
		}
		rootEco, err := coreworkspace.GetProjectByPath(currentNode.RootEcosystemPath)
		if err != nil {
			return nil, fmt.Errorf("root ecosystem not found at path: %s: %w", currentNode.RootEcosystemPath, err)
		}

		// Find the corresponding subproject in the main ecosystem
		for _, node := range s.workspaceProvider.All() {
			if node.Kind == coreworkspace.KindEcosystemSubProject &&
			   node.Name == currentNode.Name &&
			   strings.EqualFold(node.ParentEcosystemPath, rootEco.Path) {
				return node, nil
			}
		}

		// If not found, fall back to the root ecosystem
		return rootEco, nil

	case coreworkspace.KindStandaloneProjectWorktree, coreworkspace.KindEcosystemSubProjectWorktree:
		// For standalone worktrees, find the parent project
		if currentNode.ParentProjectPath != "" {
			parent, err := coreworkspace.GetProjectByPath(currentNode.ParentProjectPath)
			if err != nil {
				return nil, fmt.Errorf("parent project not found at path: %s: %w", currentNode.ParentProjectPath, err)
			}
			return parent, nil
		}
		return currentNode, nil

	case coreworkspace.KindEcosystemWorktree:
		// For ecosystem worktrees, use the root ecosystem
		if currentNode.RootEcosystemPath != "" {
			rootNode, err := coreworkspace.GetProjectByPath(currentNode.RootEcosystemPath)
			if err != nil {
				return nil, fmt.Errorf("root ecosystem not found at path: %s: %w", currentNode.RootEcosystemPath, err)
			}
			return rootNode, nil
		}
		return currentNode, nil

	default:
		// For all other cases (standalone projects, ecosystem roots, ecosystem subprojects),
		// the project is its own notebook context
		return currentNode, nil
	}
}

// buildPathsMap creates the map of note type paths for a given context using the NotebookLocator.
// Note: We always use "main" as the branch for notebook paths for consistency,
// regardless of the current branch or worktree the user is in.
func (s *Service) buildPathsMap(notebookContext *coreworkspace.WorkspaceNode) (map[string]string, error) {
	paths := make(map[string]string)
	types := []string{"current", "llm", "learn", "daily", "issues", "architecture", "todos", "quick", "archive", "prompts"}

	for _, t := range types {
		path, err := s.notebookLocator.GetNotesDir(notebookContext, t)
		if err != nil {
			return nil, fmt.Errorf("get notes dir for type %s: %w", t, err)
		}
		paths[t] = path
	}
	return paths, nil
}

// getNotePathForContext is a convenience wrapper that uses the NotebookLocator.
func (s *Service) getNotePathForContext(ctx *WorkspaceContext, noteType string) (string, error) {
	return s.notebookLocator.GetNotesDir(ctx.NotebookContextWorkspace, noteType)
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

	// Extract metadata from path
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
// Note: branch parameter is accepted for API compatibility but always uses "main"
func (s *Service) BuildNotePath(workspaceName, branch, noteType, filename string) (string, error) {
	var targetNode *coreworkspace.WorkspaceNode
	if workspaceName == "global" || workspaceName == "" {
		targetNode = &coreworkspace.WorkspaceNode{Name: "global"}
	} else {
		// Find workspace node by name from the provider
		for _, node := range s.workspaceProvider.All() {
			if node.Name == workspaceName && !node.IsWorktree() { // Find the root project
				targetNode = node
				break
			}
		}
	}

	if targetNode == nil {
		return "", fmt.Errorf("workspace not found: %s", workspaceName)
	}

	// Get the note directory using NotebookLocator
	noteDir, err := s.notebookLocator.GetNotesDir(targetNode, noteType)
	if err != nil {
		return "", fmt.Errorf("get notes dir: %w", err)
	}

	return filepath.Join(noteDir, filename), nil
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
	if s.Index != nil {
		if err := s.Index.Close(); err != nil {
			return err
		}
	}
	return nil
}

// WorkspaceContext holds current workspace information
type WorkspaceContext struct {
	CurrentWorkspace         *coreworkspace.WorkspaceNode
	NotebookContextWorkspace *coreworkspace.WorkspaceNode
	Branch                   string
	Paths                    map[string]string
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
	allWorkspaces bool
	noteType      models.NoteType
	limit         int
}

type SearchOption func(*searchOptions)

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
func (s *Service) ListAllNotesInWorkspace(ws *coreworkspace.WorkspaceNode) ([]*models.Note, error) {
	if ws.Kind != coreworkspace.KindStandaloneProject && ws.Kind != coreworkspace.KindEcosystemRoot && ws.Kind != coreworkspace.KindEcosystemSubProject {
		return nil, fmt.Errorf("listing notes across all branches is only supported for root projects, not worktrees")
	}

	// Get the root path by getting any note type directory and going up two levels
	// This works because the structure is: .../repos/{workspace}/main/{noteType}
	samplePath, err := s.notebookLocator.GetNotesDir(ws, "current")
	if err != nil {
		return nil, fmt.Errorf("get notes root: %w", err)
	}
	// Go up two levels to get to the workspace directory (repos/{workspace})
	repoNotesRoot := filepath.Dir(filepath.Dir(samplePath))

	var notes []*models.Note
	err = filepath.Walk(repoNotesRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors to continue walking
		}
		if strings.Contains(path, "/archive/") {
			return filepath.SkipDir
		}
		if !info.IsDir() && strings.HasSuffix(path, ".md") {
			note, err := ParseNote(path)
			if err == nil {
				notes = append(notes, note)
			}
		}
		return nil
	})
	return notes, err
}

// GetBranches returns all branches for a git workspace
func (s *Service) GetBranches(ws *coreworkspace.WorkspaceNode) ([]string, error) {
	if !git.IsGitRepo(ws.Path) {
		return []string{}, nil
	}

	cmd := exec.Command("git", "branch", "--format=%(refname:short)")
	cmd.Dir = ws.Path
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("list branches: %w", err)
	}

	var branches []string
	for _, line := range strings.Split(string(output), "\n") {
		branch := strings.TrimSpace(line)
		if branch != "" {
			branches = append(branches, branch)
		}
	}

	if len(branches) == 0 {
		branches = []string{"main"}
	}

	return branches, nil
}
