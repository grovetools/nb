package service

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	coreconfig "github.com/mattsolo1/grove-core/config"
	"github.com/mattsolo1/grove-core/git"
	coreworkspace "github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-core/util/pathutil"
	"github.com/mattsolo1/grove-notebook/pkg/frontmatter"
	"github.com/mattsolo1/grove-notebook/pkg/models"
	"github.com/mattsolo1/grove-notebook/pkg/tree"
	"github.com/sirupsen/logrus"
)

// CreateNoteWithContent creates a new note programmatically without opening an editor.
// This is used by the sync system to create notes for synced items.
func (s *Service) CreateNoteWithContent(
	ctx *WorkspaceContext,
	noteType models.NoteType,
	title string,
	fm *frontmatter.Frontmatter,
	body string,
) (*models.Note, error) {
	// 1. Ensure directory exists
	noteDir, err := s.getNotePathForContext(ctx, string(noteType))
	if err != nil {
		return nil, fmt.Errorf("get note path: %w", err)
	}
	if err := os.MkdirAll(noteDir, 0755); err != nil {
		return nil, fmt.Errorf("ensure directories: %w", err)
	}

	// 2. Generate filename from title
	filename := GenerateFilename(title)
	notePath := filepath.Join(noteDir, filename)

	// 3. Build complete content with frontmatter + body
	content := frontmatter.BuildContent(fm, body)

	// 4. Write file to disk
	if err := os.WriteFile(notePath, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("write note: %w", err)
	}

	// 5. Set file modification time to match frontmatter if specified
	if fm.Modified != "" {
		if modTime, err := frontmatter.ParseTimestamp(fm.Modified); err == nil {
			// Use the same time for both atime and mtime
			os.Chtimes(notePath, modTime, modTime)
		}
	}

	// 6. Parse the note and return it
	note, err := ParseNote(notePath)
	if err != nil {
		return nil, fmt.Errorf("parse created note: %w", err)
	}
	return note, nil
}

// UpdateNoteWithContent updates an existing note's content programmatically.
// This is used by the sync system to update notes when remote items change.
func (s *Service) UpdateNoteWithContent(
	notePath string,
	fm *frontmatter.Frontmatter,
	body string,
) error {
	// 1. Get original file info to preserve permissions
	info, err := os.Stat(notePath)
	if err != nil {
		return fmt.Errorf("stat original note: %w", err)
	}

	// 2. Build new content with updated frontmatter + body
	content := frontmatter.BuildContent(fm, body)

	// 3. Write back to disk
	if err := os.WriteFile(notePath, []byte(content), info.Mode()); err != nil {
		return fmt.Errorf("write updated note: %w", err)
	}

	// 4. Set file modification time to match frontmatter if specified
	if fm.Modified != "" {
		if modTime, err := frontmatter.ParseTimestamp(fm.Modified); err == nil {
			// Use the same time for both atime and mtime
			os.Chtimes(notePath, modTime, modTime)
		}
	}

	return nil
}

// DeleteNotes removes note files from the filesystem.
func (s *Service) DeleteNotes(paths []string) error {
	s.Logger.WithField("count", len(paths)).Warn("Deleting notes")

	var errs []string
	for _, path := range paths {
		if err := os.Remove(path); err != nil {
			s.Logger.WithError(err).WithField("path", path).Error("Failed to delete note")
			errs = append(errs, fmt.Sprintf("failed to delete %s: %v", path, err))
		} else {
			s.Logger.WithField("path", path).Warn("Deleted note")
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf(strings.Join(errs, "; "))
	}
	return nil
}

// MoveNotes moves notes to a new workspace and group.
func (s *Service) MoveNotes(sourcePaths []string, destWorkspace *coreworkspace.WorkspaceNode, destGroup string) ([]string, error) {
	return s.transferNotes(sourcePaths, destWorkspace, destGroup, "move")
}

// CopyNotes copies notes to a new workspace and group.
func (s *Service) CopyNotes(sourcePaths []string, destWorkspace *coreworkspace.WorkspaceNode, destGroup string) ([]string, error) {
	return s.transferNotes(sourcePaths, destWorkspace, destGroup, "copy")
}

// transferNotes is a helper for moving or copying notes.
func (s *Service) transferNotes(sourcePaths []string, destWorkspace *coreworkspace.WorkspaceNode, destGroup, mode string) ([]string, error) {
	s.Logger.WithFields(logrus.Fields{
		"operation":             mode,
		"count":                 len(sourcePaths),
		"destination_workspace": destWorkspace.Name,
		"destination_group":     destGroup,
	}).Info("Starting note transfer operation")

	var newPaths []string
	var errs []string

	for _, sourcePath := range sourcePaths {
		filename := filepath.Base(sourcePath)
		destDir, err := s.notebookLocator.GetNotesDir(destWorkspace, destGroup)
		if err != nil {
			errs = append(errs, fmt.Sprintf("failed to get dest dir for %s: %v", sourcePath, err))
			continue
		}

		if err := os.MkdirAll(destDir, 0755); err != nil {
			errs = append(errs, fmt.Sprintf("failed to create dest dir %s: %v", destDir, err))
			continue
		}

		destPath := filepath.Join(destDir, filename)
		isCopyToSameLocation := false

		// Handle filename collisions
		if _, err := os.Stat(destPath); err == nil && destPath != sourcePath {
			base := strings.TrimSuffix(filename, filepath.Ext(filename))
			ext := filepath.Ext(filename)
			timestamp := time.Now().Format("20060102150405")
			destPath = filepath.Join(destDir, fmt.Sprintf("%s-%s%s", base, timestamp, ext))

			// Check if copying to same directory
			if mode == "copy" && filepath.Dir(sourcePath) == filepath.Dir(destPath) {
				isCopyToSameLocation = true
			}
		}

		var opErr error
		if mode == "copy" {
			opErr = copyFile(sourcePath, destPath)
		} else { // move
			opErr = os.Rename(sourcePath, destPath)
			if opErr != nil {
				// Fallback to copy and delete if rename fails (e.g., cross-device)
				opErr = copyAndDelete(sourcePath, destPath)
			}
		}

		if opErr != nil {
			errs = append(errs, fmt.Sprintf("failed to %s %s: %v", mode, sourcePath, opErr))
			continue
		}

		// Update frontmatter to match the new location
		if updateErr := s.updateNoteFrontmatter(destPath, destWorkspace, destGroup, isCopyToSameLocation); updateErr != nil {
			// Log warning but don't fail the operation
			s.Logger.WithError(updateErr).WithField("path", destPath).Warn("Failed to update frontmatter")
		}

		s.Logger.WithFields(logrus.Fields{
			"source_path": sourcePath,
			"dest_path":   destPath,
			"operation":   mode,
		}).Debug("Successfully transferred note")

		newPaths = append(newPaths, destPath)
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf(strings.Join(errs, "; "))
	}
	return newPaths, nil
}

// updateNoteFrontmatter updates frontmatter fields to match the new location
func (s *Service) updateNoteFrontmatter(notePath string, destWorkspace *coreworkspace.WorkspaceNode, newType string, isCopyToSameLocation bool) error {
	content, err := os.ReadFile(notePath)
	if err != nil {
		return fmt.Errorf("read note: %w", err)
	}

	contentStr := string(content)
	fm, body, err := frontmatter.Parse(contentStr)
	if err != nil || fm == nil {
		// No frontmatter or parsing error - skip update
		return nil
	}

	// If copying to same location, update title and ID to distinguish the copy
	if isCopyToSameLocation {
		// Add "Copy" suffix to title
		if !strings.Contains(fm.Title, "Copy") {
			fm.Title = fm.Title + " Copy"
		} else {
			// If already has "Copy", add a number
			copyCount := 2
			for {
				newTitle := fmt.Sprintf("%s %d", fm.Title, copyCount)
				if newTitle != fm.Title {
					fm.Title = newTitle
					break
				}
				copyCount++
			}
		}

		// Generate new ID based on new title
		fm.ID = GenerateNoteID(fm.Title)

		// Update modified timestamp
		fm.Modified = frontmatter.FormatTimestamp(time.Now())

		// Update the first heading in the body to match new title
		bodyLines := strings.Split(body, "\n")
		for i, line := range bodyLines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "# ") {
				bodyLines[i] = "# " + fm.Title
				break
			}
		}
		body = strings.Join(bodyLines, "\n")
	}

	// Update the type field
	fm.Type = newType

	// Update tags to reflect the new type/location
	// Extract tags from the new type (e.g., "issues/bugs" -> ["issues", "bugs"])
	pathTags := frontmatter.ExtractPathTags(newType)

	// Keep the repository tag
	var repoTags []string
	if destWorkspace != nil && destWorkspace.Name != "global" {
		repoTags = []string{destWorkspace.Name}
	}

	// Merge path tags and repository tag
	fm.Tags = frontmatter.MergeTags(pathTags, repoTags)

	// Update repository, branch, and worktree fields based on destination workspace
	if destWorkspace != nil {
		fm.Repository = destWorkspace.Name

		// Get branch information if it's a git repo
		if git.IsGitRepo(destWorkspace.Path) {
			_, branch, _ := git.GetRepoInfo(destWorkspace.Path)
			fm.Branch = branch
		} else {
			fm.Branch = ""
		}

		// Set worktree name if applicable
		fm.Worktree = destWorkspace.GetWorktreeName()
	}

	// Rebuild content with updated frontmatter
	updatedContent := frontmatter.BuildContent(fm, body)

	// Write back to file
	if err := os.WriteFile(notePath, []byte(updatedContent), 0644); err != nil {
		return fmt.Errorf("write note: %w", err)
	}

	return nil
}

// Service is the core note service
type Service struct {
	workspaceProvider *coreworkspace.Provider
	notebookLocator   *coreworkspace.NotebookLocator
	Config            *Config
	CoreConfig        *coreconfig.Config
	Logger            *logrus.Entry
}

// Config holds service configuration
type Config struct {
	DataDir     string
	Editor      string
	Templates   map[string]string
	DefaultType models.NoteType
}

// New creates a new note service
func New(config *Config, provider *coreworkspace.Provider, coreCfg *coreconfig.Config, logger *logrus.Entry) (*Service, error) {
	notebookLocator := coreworkspace.NewNotebookLocator(coreCfg)

	return &Service{
		workspaceProvider: provider,
		notebookLocator:   notebookLocator,
		Config:            config,
		CoreConfig:        coreCfg,
		Logger:            logger,
	}, nil
}

// GetWorkspaceProvider returns the workspace provider
func (s *Service) GetWorkspaceProvider() *coreworkspace.Provider {
	return s.workspaceProvider
}

// GetNotebookLocator returns the notebook locator
func (s *Service) GetNotebookLocator() *coreworkspace.NotebookLocator {
	return s.notebookLocator
}

// ListNoteTypes discovers note types by scanning directories within the notes path.
// It ensures 'inbox' is always included as the default.
func (s *Service) ListNoteTypes(notebookContext *coreworkspace.WorkspaceNode) ([]models.NoteType, error) {
	types := make(map[models.NoteType]bool)
	types["inbox"] = true // Always include inbox

	// The locator's GetNotesDir with an empty type gives us the root directory for notes.
	notesRootDir, err := s.notebookLocator.GetNotesDir(notebookContext, "")
	if err != nil {
		// If we can't get the root, return the default. This can happen for new workspaces.
		return []models.NoteType{"inbox"}, nil
	}

	entries, err := os.ReadDir(notesRootDir)
	if err != nil {
		// Directory might not exist yet, which is fine. Return the default.
		if os.IsNotExist(err) {
			return []models.NoteType{"inbox"}, nil
		}
		return nil, fmt.Errorf("could not read notes directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			types[models.NoteType(entry.Name())] = true
		}
	}

	// Convert map to slice
	var result []models.NoteType
	for t := range types {
		result = append(result, t)
	}

	return result, nil
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
	if noteType == "quick" {
		filename = time.Now().Format("150405") + "-quick.md"
	} else if noteType == "daily" {
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

	// Look up user-defined note type configuration from core config
	var noteTypeConfig *coreconfig.NoteTypeConfig
	if s.CoreConfig != nil && s.CoreConfig.Notebooks != nil && s.CoreConfig.Notebooks.Definitions != nil {
		// Try to get default notebook name from rules, otherwise fall back to "default"
		defaultNotebookName := "default"
		if s.CoreConfig.Notebooks.Rules != nil && s.CoreConfig.Notebooks.Rules.Default != "" {
			defaultNotebookName = s.CoreConfig.Notebooks.Rules.Default
		}
		if notebook, ok := s.CoreConfig.Notebooks.Definitions[defaultNotebookName]; ok && notebook.Types != nil {
			noteTypeConfig = notebook.Types[string(noteType)]
		}
	}

	// Get worktree name using the proper workspace model method
	worktreeName := ""
	if currentContext.CurrentWorkspace != nil {
		worktreeName = currentContext.CurrentWorkspace.GetWorktreeName()
	}

	content := CreateNoteContent(noteType, title, currentContext.NotebookContextWorkspace.Name, currentContext.Branch, worktreeName, currentContext.CurrentWorkspace.Name, template, noteTypeConfig)

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


	// Open in editor if requested
	if opts.openEditor && s.Config.Editor != "" {
		if err := s.openInEditor(notePath); err != nil {
			s.Logger.WithError(err).Warn("Failed to open editor")
		}
	}

	s.Logger.WithFields(logrus.Fields{
		"path":      note.Path,
		"type":      note.Type,
		"title":     note.Title,
		"workspace": note.Workspace,
		"branch":    note.Branch,
	}).Info("Created new note")

	return note, nil
}

// SearchNotes searches for notes matching the query using filesystem tools.
func (s *Service) SearchNotes(ctx *WorkspaceContext, query string, options ...SearchOption) ([]*models.Note, error) {
	opts := &searchOptions{
		limit: 50,
	}
	for _, opt := range options {
		opt(opts)
	}

	// 1. Determine directories to search
	var searchDirs []string
	uniqueDirs := make(map[string]bool)

	if opts.allWorkspaces {
		allWorkspaces := s.workspaceProvider.All()
		for _, ws := range allWorkspaces {
			contextNode, err := s.findNotebookContextNode(ws)
			if err != nil {
				continue
			}
			sampleDir, err := s.notebookLocator.GetNotesDir(contextNode, "inbox")
			if err == nil {
				notesRoot := filepath.Dir(sampleDir)
				uniqueDirs[notesRoot] = true
			}
		}
	} else {
		sampleDir, err := s.notebookLocator.GetNotesDir(ctx.NotebookContextWorkspace, "inbox")
		if err != nil {
			return nil, fmt.Errorf("could not determine search directory for context: %w", err)
		}
		notesRoot := filepath.Dir(sampleDir)
		uniqueDirs[notesRoot] = true
	}

	for dir := range uniqueDirs {
		searchDirs = append(searchDirs, dir)
	}

	if len(searchDirs) == 0 {
		return []*models.Note{}, nil
	}

	// 2. Execute search command
	var cmd *exec.Cmd
	rgPath, err := exec.LookPath("rg")
	if err == nil {
		args := []string{"--glob", "*.md", "--ignore-case", "-l", query}
		args = append(args, searchDirs...)
		cmd = exec.Command(rgPath, args...)
		s.Logger.WithFields(logrus.Fields{
			"command": "rg",
			"args":    args,
			"query":   query,
		}).Debug("Executing search command")
	} else {
		// Fallback to grep
		grepPath, err := exec.LookPath("grep")
		if err != nil {
			return nil, fmt.Errorf("neither 'rg' nor 'grep' found in PATH")
		}
		args := []string{"-rli", "--include=*.md", query}
		args = append(args, searchDirs...)
		cmd = exec.Command(grepPath, args...)
		s.Logger.WithFields(logrus.Fields{
			"command": "grep",
			"args":    args,
			"query":   query,
		}).Debug("Executing search command")
	}

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// grep and rg exit with 1 if no matches are found, which is not an error for us.
			if exitErr.ExitCode() != 1 {
				return nil, fmt.Errorf("search command failed: %w, stderr: %s", err, exitErr.Stderr)
			}
		} else {
			return nil, fmt.Errorf("search command failed: %w", err)
		}
	}

	// 3. Parse results
	var results []*models.Note
	filePaths := strings.Split(string(output), "\n")

	for _, path := range filePaths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}

		note, err := ParseNote(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not parse note %s: %v\n", path, err)
			continue
		}

		// 4. In-memory filtering by type
		if opts.noteType != "" && note.Type != opts.noteType {
			continue
		}

		results = append(results, note)
	}

	// 5. Apply limit
	if len(results) > opts.limit {
		results = results[:opts.limit]
	}

	s.Logger.WithFields(logrus.Fields{
		"query":         query,
		"results_count": len(results),
	}).Debug("Search completed")

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
func (s *Service) ListAllNotes(ctx *WorkspaceContext, includeArchived bool, includeArtifacts bool) ([]*models.Note, error) {
	// Get all content directories for this workspace
	contentDirs, err := s.notebookLocator.GetAllContentDirs(ctx.NotebookContextWorkspace)
	if err != nil {
		return nil, fmt.Errorf("get content directories: %w", err)
	}

	var notes []*models.Note
	processedPaths := make(map[string]struct{})

	// Walk each content directory
	for _, contentDir := range contentDirs {
		// Skip if directory doesn't exist
		if _, err := os.Stat(contentDir.Path); err != nil {
			continue
		}

		err = filepath.Walk(contentDir.Path, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // Skip errors
			}

			// De-duplication: check if we've already processed this canonical path
			canonicalPath, err := pathutil.NormalizeForLookup(path)
			if err != nil {
				return nil // Skip if we cannot normalize the path
			}
			if _, seen := processedPaths[canonicalPath]; seen {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			processedPaths[canonicalPath] = struct{}{}

			// Skip archive directories if not requested
			if !includeArchived {
				if strings.Contains(path, "/.archive/") || strings.Contains(path, "/archive/") {
					if info.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
			}

			// Ignore common dotfiles, but not special dot-directories like .archive
			if !info.IsDir() && strings.HasPrefix(info.Name(), ".") {
				return nil
			}

			if !info.IsDir() {
				var note *models.Note
				var err error
				if strings.HasSuffix(path, ".md") {
					note, err = ParseNote(path)
				} else {
					// Handle non-markdown files as generic file notes
					note, err = ParseGenericFile(path)
				}

				if err == nil {
					// Enrich with context if not already set by path parsing
					if note.Workspace == "" {
						note.Workspace = ctx.NotebookContextWorkspace.Name
					}
					if note.Branch == "" {
						note.Branch = ctx.Branch
					}

					// Set Group from directory path for grouping in UI
					relPath, _ := filepath.Rel(contentDir.Path, path)
					parts := strings.Split(filepath.ToSlash(relPath), "/")

					switch contentDir.Type {
					case "plans":
						if len(parts) > 2 {
							// This is inside a nested subdirectory: "plans/<dir>/<subdir>"
							// Preserve the full path for archived plans like "plans/.archive/planname"
							note.Group = "plans/" + filepath.Join(parts[0], parts[1])
						} else if len(parts) > 1 {
							// This is inside a plan subdirectory: "plans/<planname>"
							note.Group = "plans/" + parts[0]
						} else {
							// This is a top-level plan file (shouldn't happen but handle it)
							note.Group = "plans"
						}
						// Set Type as plan
						if note.Type == "" {
							note.Type = "plan"
						}

					case "chats":
						if len(parts) > 2 {
							// This is inside a nested subdirectory: "chats/<dir>/<subdir>"
							note.Group = "chats/" + filepath.Join(parts[0], parts[1])
						} else if len(parts) > 1 {
							// This is inside a chat subdirectory: "chats/<chatname>"
							note.Group = "chats/" + parts[0]
						} else {
							note.Group = "chats"
						}
						if note.Type == "" {
							note.Type = "chat"
						}

					case "notes":
						if len(parts) > 1 {
							note.Group = strings.Join(parts[:len(parts)-1], "/")
						} else if len(parts) == 1 {
							note.Group = "quick"
						}
						// Set Type from directory if not already set from frontmatter (for backwards compatibility)
						if note.Type == "" {
							note.Type = models.NoteType(note.Group)
						}
					}

					notes = append(notes, note)
				}
			}
			return nil
		})
	}

	return notes, nil
}

// ListAllItems lists all files as generic Items in the specified workspace context.
func (s *Service) ListAllItems(ctx *WorkspaceContext, includeArchived bool, includeArtifacts bool) ([]*tree.Item, error) {
	// Get all content directories for this workspace
	contentDirs, err := s.notebookLocator.GetAllContentDirs(ctx.NotebookContextWorkspace)
	if err != nil {
		return nil, fmt.Errorf("get content directories: %w", err)
	}

	var items []*tree.Item
	processedPaths := make(map[string]struct{})

	// Walk each content directory
	for _, contentDir := range contentDirs {
		// Skip if directory doesn't exist
		if _, err := os.Stat(contentDir.Path); err != nil {
			continue
		}

		err = filepath.Walk(contentDir.Path, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // Skip errors
			}

			// De-duplication: check if we've already processed this canonical path
			canonicalPath, err := pathutil.NormalizeForLookup(path)
			if err != nil {
				return nil // Skip if we cannot normalize the path
			}
			if _, seen := processedPaths[canonicalPath]; seen {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			processedPaths[canonicalPath] = struct{}{}

			// Skip archive directories if not requested
			if !includeArchived && (info.Name() == ".archive" || info.Name() == "archive") {
				return filepath.SkipDir
			}
			// Skip artifacts directories if not requested
			if !includeArtifacts && info.Name() == ".artifacts" {
				return filepath.SkipDir
			}

			// Ignore common dotfiles
			if !info.IsDir() && strings.HasPrefix(info.Name(), ".") {
				return nil
			}

			if !info.IsDir() {
				item, err := s.newItemFromFile(path, info)
				if err == nil {
					// Enrich item with workspace context
					item.Metadata["Workspace"] = ctx.NotebookContextWorkspace.Name
					item.Metadata["Branch"] = ctx.Branch

					// Set Group from directory path for grouping in UI
					relPath, _ := filepath.Rel(contentDir.Path, path)
					parts := strings.Split(filepath.ToSlash(relPath), "/")
					group := ""
					if len(parts) > 1 {
						group = strings.Join(parts[:len(parts)-1], "/")
					}
					item.Metadata["Group"] = group

					items = append(items, item)
				}
			}
			return nil
		})
	}

	return items, nil
}

// ListAllGlobalNotes lists all notes in the global workspace (all directories)
func (s *Service) ListAllGlobalNotes(includeArchived bool, includeArtifacts bool) ([]*models.Note, error) {
	ctx, err := s.GetWorkspaceContext("global")
	if err != nil {
		return nil, fmt.Errorf("get global workspace context: %w", err)
	}
	return s.ListAllNotes(ctx, includeArchived, includeArtifacts)
}

// ListAllGlobalItems lists all items in the global workspace (all directories)
func (s *Service) ListAllGlobalItems(includeArchived bool, includeArtifacts bool) ([]*tree.Item, error) {
	ctx, err := s.GetWorkspaceContext("global")
	if err != nil {
		return nil, fmt.Errorf("get global workspace context: %w", err)
	}
	return s.ListAllItems(ctx, includeArchived, includeArtifacts)
}

// ListGlobalNotes lists notes in the global workspace
func (s *Service) ListGlobalNotes(noteType models.NoteType) ([]*models.Note, error) {
	ctx, err := s.GetWorkspaceContext("global")
	if err != nil {
		return nil, fmt.Errorf("get global workspace context: %w", err)
	}
	return s.ListNotes(ctx, noteType)
}

// ArchiveNotes moves notes to a .archive subdirectory within their current directory.
func (s *Service) ArchiveNotes(ctx *WorkspaceContext, paths []string) error {
	s.Logger.WithField("count", len(paths)).Info("Archiving notes")
	for _, path := range paths {
		// 1. Get the parent directory of the note file.
		noteDir := filepath.Dir(path)
		filename := filepath.Base(path)

		// 2. Define the path for the archive subdirectory.
		archiveDir := filepath.Join(noteDir, ".archive")

		// 3. Ensure this .archive directory exists.
		if err := os.MkdirAll(archiveDir, 0755); err != nil {
			return fmt.Errorf("failed to create archive directory for %s: %w", path, err)
		}

		// 4. Define the destination path.
		dest := filepath.Join(archiveDir, filename)

		// 5. Add collision handling.
		if _, err := os.Stat(dest); err == nil {
			// File exists, append timestamp to prevent data loss.
			ext := filepath.Ext(filename)
			base := strings.TrimSuffix(filename, ext)
			timestamp := time.Now().Format("20060102150405")
			newFilename := fmt.Sprintf("%s-%s%s", base, timestamp, ext)
			dest = filepath.Join(archiveDir, newFilename)
		}

		// 6. Move the note.
		if err := os.Rename(path, dest); err != nil {
			s.Logger.WithError(err).WithField("path", path).Error("Failed to move note to archive")
			return fmt.Errorf("failed to move %s to archive: %w", path, err)
		}
		s.Logger.WithFields(logrus.Fields{
			"source_path":  path,
			"archive_path": dest,
		}).Debug("Archived note")
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
		// Try to detect if we're in a notebooks workspace directory
		// Pattern: <notebooks_root>/nb/workspaces/<workspace_name>/...
		if ws := s.extractWorkspaceFromNotebooksPath(CWD); ws != nil {
			currentWorkspace = ws
		} else {
			// Fallback to global context if not in a known workspace
			return s.GetWorkspaceContext("global")
		}
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

	ctx := &WorkspaceContext{
		CurrentWorkspace:         currentWorkspace,
		NotebookContextWorkspace: notebookContextWorkspace,
		Branch:                   branch,
		Paths:                    paths,
	}

	s.Logger.WithFields(logrus.Fields{
		"start_path":                 CWD,
		"current_workspace_name":     ctx.CurrentWorkspace.Name,
		"current_workspace_path":     ctx.CurrentWorkspace.Path,
		"current_workspace_kind":     ctx.CurrentWorkspace.Kind,
		"notebook_context_name":      ctx.NotebookContextWorkspace.Name,
		"notebook_context_path":      ctx.NotebookContextWorkspace.Path,
		"notebook_context_notebook":  ctx.NotebookContextWorkspace.NotebookName,
	}).Debug("Resolved workspace context")

	return ctx, nil
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

// extractWorkspaceFromNotebooksPath attempts to extract a workspace from a notebooks directory path.
// Pattern: <notebooks_root>/workspaces/<workspace_name>/...
// Returns nil if the path doesn't match the pattern or workspace not found.
func (s *Service) extractWorkspaceFromNotebooksPath(path string) *coreworkspace.WorkspaceNode {
	// Get the notebook root directory from config
	if s.CoreConfig == nil || s.CoreConfig.Notebooks == nil || s.CoreConfig.Notebooks.Definitions == nil {
		return nil
	}

	// Find the default notebook configuration
	defaultNotebookName := "default"
	if s.CoreConfig.Notebooks.Rules != nil && s.CoreConfig.Notebooks.Rules.Default != "" {
		defaultNotebookName = s.CoreConfig.Notebooks.Rules.Default
	}

	notebook, ok := s.CoreConfig.Notebooks.Definitions[defaultNotebookName]
	if !ok || notebook == nil || notebook.RootDir == "" {
		return nil
	}

	notebooksRoot := notebook.RootDir

	// Expand ~ in the notebooks root path
	if strings.HasPrefix(notebooksRoot, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			notebooksRoot = filepath.Join(home, notebooksRoot[2:])
		}
	}

	// Clean the paths for comparison
	cleanPath := filepath.Clean(path)
	cleanRoot := filepath.Clean(notebooksRoot)

	// Check if path is under notebooks root
	if !strings.HasPrefix(cleanPath, cleanRoot) {
		return nil
	}

	// Extract the relative path from notebooks root
	relPath, err := filepath.Rel(cleanRoot, cleanPath)
	if err != nil {
		return nil
	}

	// Check if it matches the pattern: workspaces/<workspace_name>/...
	parts := strings.Split(relPath, string(filepath.Separator))
	if len(parts) < 2 || parts[0] != "workspaces" {
		return nil
	}

	workspaceName := parts[1]

	// Look up the workspace by name
	for _, ws := range s.workspaceProvider.All() {
		if ws.Name == workspaceName {
			return ws
		}
	}

	return nil
}

// buildPathsMap creates the map of note type paths for a given context using the NotebookLocator.
// Note: We always use "main" as the branch for notebook paths for consistency,
// regardless of the current branch or worktree the user is in.
func (s *Service) buildPathsMap(notebookContext *coreworkspace.WorkspaceNode) (map[string]string, error) {
	paths := make(map[string]string)

	// Get configured note types
	noteTypes, err := s.ListNoteTypes(notebookContext)
	if err != nil {
		// Fallback to a basic set if there's an error
		noteTypes = []models.NoteType{"inbox", "quick", "learn"}
	}

	for _, t := range noteTypes {
		path, err := s.notebookLocator.GetNotesDir(notebookContext, string(t))
		if err != nil {
			return nil, fmt.Errorf("get notes dir for type %s: %w", t, err)
		}
		paths[string(t)] = path
	}

	// Add plans directory
	plansPath, err := s.notebookLocator.GetPlansDir(notebookContext)
	if err != nil {
		return nil, fmt.Errorf("get plans dir: %w", err)
	}
	paths["plans"] = plansPath

	// Add templates directory
	templatesPath, err := s.notebookLocator.GetTemplatesDir(notebookContext)
	if err != nil {
		return nil, fmt.Errorf("get templates dir: %w", err)
	}
	paths["templates"] = templatesPath

	// Add recipes directory
	recipesPath, err := s.notebookLocator.GetRecipesDir(notebookContext)
	if err != nil {
		return nil, fmt.Errorf("get recipes dir: %w", err)
	}
	paths["recipes"] = recipesPath

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

	s.Logger.WithField("path", path).Info("Updated note content")

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

// Close closes the service
func (s *Service) Close() error {
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

func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	if err != nil {
		return err
	}

	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	return os.Chmod(dst, srcInfo.Mode())
}

func copyAndDelete(src, dst string) error {
	if err := copyFile(src, dst); err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}
	if err := os.Remove(src); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to remove source file: %v\n", err)
	}
	return nil
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
	samplePath, err := s.notebookLocator.GetNotesDir(ws, "inbox")
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
