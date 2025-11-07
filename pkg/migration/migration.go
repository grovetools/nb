package migration

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	coreworkspace "github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-notebook/pkg/models"
	"github.com/mattsolo1/grove-notebook/pkg/search"
)

func Migrate(basePath string, options MigrationOptions, output io.Writer) (*MigrationReport, error) {
	if output == nil {
		output = os.Stdout
	}

	migrator := NewMigrator(options, basePath, output)

	var paths []string

	switch {
	case options.Scope.Context != "":
		contextPath := filepath.Join(basePath, options.Scope.Context)
		if err := filepath.Walk(contextPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && strings.HasSuffix(path, ".md") {
				paths = append(paths, path)
			}
			return nil
		}); err != nil {
			return migrator.GetReport(), fmt.Errorf("failed to walk context directory: %w", err)
		}

	case options.Scope.Workspace != "":
		workspacePath := filepath.Join(basePath, "repos", options.Scope.Workspace)
		if err := filepath.Walk(workspacePath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && strings.HasSuffix(path, ".md") {
				paths = append(paths, path)
			}
			return nil
		}); err != nil {
			return migrator.GetReport(), fmt.Errorf("failed to walk workspace directory: %w", err)
		}

	case options.Scope.Global:
		globalPath := filepath.Join(basePath, "global")
		if err := filepath.Walk(globalPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && strings.HasSuffix(path, ".md") {
				paths = append(paths, path)
			}
			return nil
		}); err != nil {
			return migrator.GetReport(), fmt.Errorf("failed to walk global directory: %w", err)
		}

	case options.Scope.All:
		if err := filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && strings.HasSuffix(path, ".md") {
				paths = append(paths, path)
			}
			return nil
		}); err != nil {
			return migrator.GetReport(), fmt.Errorf("failed to walk all directories: %w", err)
		}

	default:
		return migrator.GetReport(), fmt.Errorf("no scope specified")
	}

	migrator.report.TotalFiles = len(paths)

	for _, path := range paths {
		if err := migrator.MigrateFile(path); err != nil {
			if options.Verbose {
				fmt.Fprintf(output, "✗ Error processing %s: %v\n", path, err)
			}
		}
	}

	migrator.Complete()

	return migrator.GetReport(), nil
}

func MigrateFile(filePath, basePath string, options MigrationOptions, output io.Writer) error {
	if output == nil {
		output = os.Stdout
	}

	migrator := NewMigrator(options, basePath, output)
	migrator.report.TotalFiles = 1

	err := migrator.MigrateFile(filePath)
	migrator.Complete()

	return err
}

func AnalyzeFile(filePath, basePath string) ([]MigrationIssue, error) {
	analyzer := NewAnalyzer(basePath)
	return analyzer.AnalyzeNote(filePath)
}

// StructuralMigration performs migration from old repos/{workspace}/{branch} structure
// to new notebooks/{workspace}/notes/{noteType} structure
type StructuralMigration struct {
	basePath        string
	locator         *coreworkspace.NotebookLocator
	index           *search.Index
	provider        *coreworkspace.Provider
	options         MigrationOptions
	report          *MigrationReport
	output          io.Writer
}

// NewStructuralMigration creates a new structural migration instance
func NewStructuralMigration(basePath string, locator *coreworkspace.NotebookLocator,
	index *search.Index, provider *coreworkspace.Provider, options MigrationOptions, output io.Writer) *StructuralMigration {
	return &StructuralMigration{
		basePath: basePath,
		locator:  locator,
		index:    index,
		provider: provider,
		options:  options,
		report:   NewMigrationReport(),
		output:   output,
	}
}

// MigrateStructure performs the structural migration
func (sm *StructuralMigration) MigrateStructure() error {
	// Walk the old repos directory
	oldReposPath := filepath.Join(sm.basePath, "repos")

	if _, err := os.Stat(oldReposPath); os.IsNotExist(err) {
		return fmt.Errorf("old repos directory not found: %s", oldReposPath)
	}

	var filesToMigrate []fileToMigrate

	// Discover all files to migrate
	err := filepath.Walk(oldReposPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}

		// Parse workspace/branch/noteType from path
		workspace, branch, noteType, err := sm.parseOldPath(path, oldReposPath)
		if err != nil {
			if sm.options.Verbose {
				fmt.Fprintf(sm.output, "⚠ Skipping %s: %v\n", path, err)
			}
			sm.report.SkippedFiles++
			return nil
		}

		filesToMigrate = append(filesToMigrate, fileToMigrate{
			oldPath:   path,
			workspace: workspace,
			branch:    branch,
			noteType:  noteType,
		})

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk repos directory: %w", err)
	}

	sm.report.TotalFiles = len(filesToMigrate)

	// Migrate each file
	for _, ftm := range filesToMigrate {
		if err := sm.migrateFile(ftm); err != nil {
			sm.report.AddError(ftm.oldPath, err)
			if sm.options.Verbose {
				fmt.Fprintf(sm.output, "✗ Error migrating %s: %v\n", ftm.oldPath, err)
			}
		}
	}

	sm.report.Complete()
	return nil
}

type fileToMigrate struct {
	oldPath   string
	workspace string
	branch    string
	noteType  string
}

// parseOldPath extracts workspace, branch, and noteType from old directory structure
// Expected format: {basePath}/repos/{workspace}/{branch}/{noteType}/...
func (sm *StructuralMigration) parseOldPath(fullPath, reposPath string) (workspace, branch, noteType string, err error) {
	relPath, err := filepath.Rel(reposPath, fullPath)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to get relative path: %w", err)
	}

	parts := strings.Split(relPath, string(filepath.Separator))
	if len(parts) < 3 {
		return "", "", "", fmt.Errorf("path does not match expected structure: %s", relPath)
	}

	workspace = parts[0]
	branch = parts[1]
	noteType = parts[2]

	return workspace, branch, noteType, nil
}

// migrateFile migrates a single file from old to new structure
func (sm *StructuralMigration) migrateFile(ftm fileToMigrate) error {
	sm.report.ProcessedFiles++

	// Read file content
	content, err := os.ReadFile(ftm.oldPath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Parse frontmatter
	fm, bodyContent, _ := ParseFrontmatter(string(content))
	if fm == nil {
		fm = &Frontmatter{
			Aliases: []string{},
			Tags:    []string{},
		}
		if !strings.HasPrefix(bodyContent, "# ") {
			bodyContent = string(content)
		}
	}

	// Backfill repository and branch if missing
	modified := false
	if fm.Repository == "" {
		fm.Repository = ftm.workspace
		modified = true
	}
	if fm.Branch == "" {
		fm.Branch = ftm.branch
		modified = true
	}

	// Get file stat for preserving timestamps
	stat, err := os.Stat(ftm.oldPath)
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}

	// Ensure other required fields are populated
	if fm.ID == "" {
		fm.ID = sm.generateID(ftm.oldPath)
		modified = true
	}
	if fm.Created == "" {
		fm.Created = FormatTimestamp(stat.ModTime())
		modified = true
	}
	if fm.Modified == "" {
		fm.Modified = FormatTimestamp(stat.ModTime())
		modified = true
	}

	// Find workspace node for new path determination
	workspaceNode := sm.findWorkspaceNode(ftm.workspace)
	if workspaceNode == nil {
		return fmt.Errorf("workspace not found: %s", ftm.workspace)
	}

	// Determine new path using NotebookLocator
	newDir, err := sm.locator.GetNotesDir(workspaceNode, ftm.noteType)
	if err != nil {
		return fmt.Errorf("failed to get notes dir: %w", err)
	}

	// Ensure new directory exists
	if err := os.MkdirAll(newDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Determine new filename
	filename := filepath.Base(ftm.oldPath)
	newPath := filepath.Join(newDir, filename)

	// Handle collision
	if _, err := os.Stat(newPath); err == nil && newPath != ftm.oldPath {
		// File already exists at destination, append branch name to avoid collision
		ext := filepath.Ext(filename)
		base := strings.TrimSuffix(filename, ext)
		newFilename := fmt.Sprintf("%s-%s%s", base, ftm.branch, ext)
		newPath = filepath.Join(newDir, newFilename)

		// If still exists, add a counter
		if _, err := os.Stat(newPath); err == nil {
			for i := 2; ; i++ {
				newFilename = fmt.Sprintf("%s-%s-%d%s", base, ftm.branch, i, ext)
				newPath = filepath.Join(newDir, newFilename)
				if _, err := os.Stat(newPath); os.IsNotExist(err) {
					break
				}
			}
		}

		if sm.options.Verbose {
			fmt.Fprintf(sm.output, "⚠ Collision: %s -> %s\n", filename, filepath.Base(newPath))
		}
	}

	// Build new content with updated frontmatter
	newContent := BuildContentWithFrontmatter(fm, bodyContent)

	if sm.options.DryRun {
		if sm.options.Verbose {
			fmt.Fprintf(sm.output, "Would migrate: %s -> %s\n", ftm.oldPath, newPath)
			if modified {
				fmt.Fprintf(sm.output, "  - Backfilled: repository=%s, branch=%s\n", fm.Repository, fm.Branch)
			}
		}
		sm.report.MigratedFiles++
		return nil
	}

	// Write to new location
	if err := os.WriteFile(newPath, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	// Preserve timestamps
	if err := os.Chtimes(newPath, stat.ModTime(), stat.ModTime()); err != nil {
		// Non-fatal, just log
		if sm.options.Verbose {
			fmt.Fprintf(sm.output, "⚠ Failed to preserve timestamps for %s\n", newPath)
		}
	}

	// Remove old file
	if err := os.Remove(ftm.oldPath); err != nil {
		return fmt.Errorf("failed to remove old file: %w", err)
	}

	// Re-index the note
	if sm.index != nil {
		note := &models.Note{
			Path:      newPath,
			Workspace: ftm.workspace,
			Branch:    ftm.branch,
			Type:      models.NoteType(ftm.noteType),
			Title:     fm.Title,
			Content:   bodyContent,
		}
		if err := sm.index.IndexNote(note); err != nil {
			// Non-fatal, just log
			if sm.options.Verbose {
				fmt.Fprintf(sm.output, "⚠ Failed to index %s: %v\n", newPath, err)
			}
		}
	}

	sm.report.MigratedFiles++
	if modified {
		sm.report.IssuesFixed++
	}

	if sm.options.Verbose {
		fmt.Fprintf(sm.output, "✓ Migrated: %s -> %s\n", ftm.oldPath, newPath)
	}

	return nil
}

// findWorkspaceNode finds a workspace node by name
func (sm *StructuralMigration) findWorkspaceNode(workspaceName string) *coreworkspace.WorkspaceNode {
	if sm.provider == nil {
		return nil
	}

	for _, node := range sm.provider.All() {
		if node.Name == workspaceName {
			return node
		}
	}

	return nil
}

// generateID generates a note ID based on file path
func (sm *StructuralMigration) generateID(filePath string) string {
	filename := filepath.Base(filePath)
	filenameStem := strings.TrimSuffix(filename, filepath.Ext(filename))

	idPattern := `^\d{8}-\d{6}`
	if matched, _ := filepath.Match(idPattern, filenameStem); matched {
		return filenameStem
	}

	// Generate new ID from current time
	return fmt.Sprintf("%s-%s",
		sm.report.StartTime.Format("20060102"),
		sm.report.StartTime.Format("150405"))
}

// GetReport returns the migration report
func (sm *StructuralMigration) GetReport() *MigrationReport {
	return sm.report
}
