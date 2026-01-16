package migration

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/grovetools/core/fs"
	coreworkspace "github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/nb/pkg/frontmatter"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

func Migrate(basePath string, options MigrationOptions, output io.Writer, logger *logrus.Entry) (*MigrationReport, error) {
	if output == nil {
		output = os.Stdout
	}

	migrator := NewMigrator(options, basePath, output, logger)

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
				fmt.Fprintf(output, "x Error processing %s: %v\n", path, err)
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

	migrator := NewMigrator(options, basePath, output, nil)
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
	sourcePath      string
	targetPath      string
	globalRoot      string
	locator         *coreworkspace.NotebookLocator
	provider        *coreworkspace.Provider
	options         MigrationOptions
	report          *MigrationReport
	output          io.Writer
	isCopyOnly      bool
}

// NewStructuralMigration creates a new structural migration instance
func NewStructuralMigration(sourcePath, targetPath, globalRoot string, locator *coreworkspace.NotebookLocator,
	provider *coreworkspace.Provider, options MigrationOptions, isCopyOnly bool, output io.Writer) *StructuralMigration {
	return &StructuralMigration{
		sourcePath: sourcePath,
		targetPath: targetPath,
		globalRoot: globalRoot,
		locator:    locator,
		provider:   provider,
		options:    options,
		isCopyOnly: isCopyOnly,
		report:     NewMigrationReport(),
		output:     output,
	}
}

// MigrateStructure performs the structural migration
func (sm *StructuralMigration) MigrateStructure() error {
	oldReposPath := filepath.Join(sm.sourcePath, "repos")
	oldGlobalPath := filepath.Join(sm.sourcePath, "global")

	var noteFilesToMigrate []fileToMigrate
	var planDirsToMigrate []planDirToMigrate

	// Phase 1: Discover all entities to migrate (notes and plan directories)
	for _, rootPath := range []string{oldReposPath, oldGlobalPath} {
		if _, err := os.Stat(rootPath); os.IsNotExist(err) {
			if sm.options.Verbose {
				fmt.Fprintf(sm.output, "Skipping non-existent directory: %s\n", rootPath)
			}
			continue
		}

		err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// Entity detection logic
			if info.IsDir() {
				dirName := info.Name()
				parentDirName := filepath.Base(filepath.Dir(path))
				// Is this a plan directory? (e.g., .../plans/my-plan or .../archive/my-plan)
				if parentDirName == "plans" {
					// All subdirectories of plans/ are plan directories
					planDirsToMigrate = append(planDirsToMigrate, planDirToMigrate{path: path, isArchived: false})
					return filepath.SkipDir // We handle the whole directory as one unit
				}
				if parentDirName == "archive" {
					// Check if this is an archived plan (has .grove-plan.yml) or just an archived note type directory
					if sm.isPlanDirectory(path) {
						planDirsToMigrate = append(planDirsToMigrate, planDirToMigrate{path: path, isArchived: true})
						return filepath.SkipDir // We handle the whole directory as one unit
					}
					// Otherwise, it's just an archived note type directory - let it be processed as notes
				}
				// Skip special directories we handle at a higher level
				if dirName == "plans" || dirName == "archive" {
					return nil
				}
			} else { // It's a file
				if !strings.HasSuffix(info.Name(), ".md") {
					return nil // Not a note
				}

				// If the file is inside a plan directory, skip it (plans are migrated as directories)
				// Note: We only check /plans/ because archived plans return filepath.SkipDir above,
				// so their files won't be visited. Files in archive/noteType/ directories should
				// be processed as regular notes.
				if strings.Contains(filepath.ToSlash(path), "/plans/") {
					return nil
				}

				workspace, branch, noteType, isArchived, parseErr := sm.parseLegacyPath(path, rootPath)
				if parseErr != nil {
					if sm.options.Verbose {
						fmt.Fprintf(sm.output, "WARNING: Skipping %s: %v\n", path, parseErr)
					}
					sm.report.SkippedFiles++
					return nil
				}
				noteFilesToMigrate = append(noteFilesToMigrate, fileToMigrate{
					oldPath:    path,
					workspace:  workspace,
					branch:     branch,
					noteType:   noteType,
					isArchived: isArchived,
				})
			}
			return nil
		})

		if err != nil {
			return fmt.Errorf("failed to walk directory %s: %w", rootPath, err)
		}
	}

	if len(noteFilesToMigrate) == 0 && len(planDirsToMigrate) == 0 {
		fmt.Fprintln(sm.output, "No legacy files or plans found to migrate.")
		sm.report.Complete()
		return nil
	}

	sm.report.TotalFiles = len(noteFilesToMigrate) + len(planDirsToMigrate)

	// Phase 2.1: Migrate plan directories
	for _, ptm := range planDirsToMigrate {
		if err := sm.migratePlanDirectory(ptm.path, ptm.isArchived); err != nil {
			sm.report.AddError(ptm.path, err)
			if sm.options.Verbose {
				fmt.Fprintf(sm.output, "x Error migrating plan directory %s: %v\n", ptm.path, err)
			}
		}
	}

	// Phase 2.2: Migrate individual note files
	for _, ftm := range noteFilesToMigrate {
		if err := sm.migrateFile(ftm); err != nil {
			sm.report.AddError(ftm.oldPath, err)
			if sm.options.Verbose {
				fmt.Fprintf(sm.output, "x Error migrating note %s: %v\n", ftm.oldPath, err)
			}
		}
	}

	// Phase 3: Clean up old directories if not a dry run and not copy-only
	if !sm.isCopyOnly && !sm.options.DryRun && sm.report.FailedFiles == 0 {
		if sm.options.Verbose {
			fmt.Fprintln(sm.output, "Migration successful. Cleaning up old directories...")
		}
		if err := os.RemoveAll(oldReposPath); err != nil {
			fmt.Fprintf(sm.output, "Warning: failed to remove old repos directory: %v\n", err)
		}
		if err := os.RemoveAll(oldGlobalPath); err != nil {
			fmt.Fprintf(sm.output, "Warning: failed to remove old global directory: %v\n", err)
		}
	} else if !sm.isCopyOnly && !sm.options.DryRun && sm.report.FailedFiles > 0 {
		fmt.Fprintf(sm.output, "Migration finished with %d errors. Old directories were not removed.\n", sm.report.FailedFiles)
	}

	sm.report.Complete()
	return nil
}

type fileToMigrate struct {
	oldPath    string
	workspace  string
	branch     string
	noteType   string
	isArchived bool
}

type planDirToMigrate struct {
	path       string
	isArchived bool
}

// conservativelyUpdateFrontmatter updates only specific frontmatter fields while preserving all others
func (sm *StructuralMigration) conservativelyUpdateFrontmatter(content string, ftm fileToMigrate, stat os.FileInfo) (string, bool, error) {
	// Extract frontmatter using regex
	frontmatterPattern := regexp.MustCompile(`(?s)^---\n(.*?)\n---\n(.*)`)
	matches := frontmatterPattern.FindStringSubmatch(content)

	if len(matches) != 3 {
		// No frontmatter - return content as-is
		return content, false, nil
	}

	frontmatterStr := matches[1]
	bodyContent := matches[2]

	// Parse into a map to preserve ALL fields
	var fmMap map[string]interface{}
	if err := yaml.Unmarshal([]byte(frontmatterStr), &fmMap); err != nil {
		return content, false, fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	modified := false

	// Only add/update specific fields if they're missing
	if _, exists := fmMap["repository"]; !exists && ftm.workspace != "global" {
		fmMap["repository"] = ftm.workspace
		modified = true
	}
	if _, exists := fmMap["branch"]; !exists && ftm.branch != "" {
		fmMap["branch"] = ftm.branch
		modified = true
	}
	if _, exists := fmMap["id"]; !exists {
		fmMap["id"] = sm.generateID(ftm.oldPath)
		modified = true
	}
	if _, exists := fmMap["created"]; !exists {
		fmMap["created"] = frontmatter.FormatTimestamp(stat.ModTime())
		modified = true
	}
	if _, exists := fmMap["modified"]; !exists {
		fmMap["modified"] = frontmatter.FormatTimestamp(stat.ModTime())
		modified = true
	}

	if !modified {
		return content, false, nil
	}

	// Marshal back to YAML, preserving all fields
	updatedFM, err := yaml.Marshal(fmMap)
	if err != nil {
		return content, false, fmt.Errorf("failed to marshal frontmatter: %w", err)
	}

	// Rebuild the content
	newContent := "---\n" + string(updatedFM) + "---\n" + bodyContent
	return newContent, true, nil
}

// parseLegacyPath extracts metadata from old directory structures.
// It handles both `repos/{workspace}/{branch}` and `global` layouts.
func (sm *StructuralMigration) parseLegacyPath(fullPath, rootPath string) (workspace, branch, noteType string, isArchived bool, err error) {
	relPath, err := filepath.Rel(rootPath, fullPath)
	if err != nil {
		return "", "", "", false, fmt.Errorf("failed to get relative path: %w", err)
	}

	parts := strings.Split(filepath.ToSlash(relPath), "/")
	filenameIndex := len(parts) - 1

	if strings.Contains(filepath.ToSlash(rootPath), "/repos") {
		if len(parts) < 4 { // workspace, branch, noteType, filename
			return "", "", "", false, fmt.Errorf("repos path does not match expected structure: %s", relPath)
		}
		workspace = parts[0]
		branch = parts[1]
		parts = parts[2:filenameIndex]
	} else if strings.Contains(filepath.ToSlash(rootPath), "/global") {
		if len(parts) < 2 { // noteType, filename
			return "", "", "", false, fmt.Errorf("global path does not match expected structure: %s", relPath)
		}
		workspace = "global"
		branch = ""
		parts = parts[0:filenameIndex]
	} else {
		return "", "", "", false, fmt.Errorf("unknown root path for parsing: %s", rootPath)
	}

	var noteTypeParts []string
	for _, part := range parts {
		if part == "archive" {
			isArchived = true
		} else {
			noteTypeParts = append(noteTypeParts, part)
		}
	}

	noteType = strings.Join(noteTypeParts, "/")
	if noteType == "" {
		noteType = "inbox" // Default if note is at the root of a branch/global dir
	}

	return workspace, branch, noteType, isArchived, nil
}

// migrateFile migrates a single file from old to new structure
func (sm *StructuralMigration) migrateFile(ftm fileToMigrate) error {
	sm.report.ProcessedFiles++

	// Read file content
	content, err := os.ReadFile(ftm.oldPath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Get file stat for preserving timestamps
	stat, err := os.Stat(ftm.oldPath)
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}

	// Conservatively update frontmatter - preserve all existing fields
	newContent, modified, err := sm.conservativelyUpdateFrontmatter(string(content), ftm, stat)
	if err != nil {
		return fmt.Errorf("failed to update frontmatter: %w", err)
	}

	// Build new path directly using the new centralized structure
	// New structure: {targetPath}/workspaces/{workspace}/{noteType}[/.archive]
	var newDir string
	if ftm.workspace == "global" {
		// Global notes go to {globalRoot}/{noteType}
		globalRoot, err := sm.getGlobalNotebookRoot()
		if err != nil {
			return fmt.Errorf("failed to get global notebook root: %w", err)
		}
		newDir = filepath.Join(globalRoot, ftm.noteType)
	} else {
		// Regular workspace notes go to {targetPath}/workspaces/{workspace}/{noteType}
		newDir = filepath.Join(sm.targetPath, "workspaces", ftm.workspace, ftm.noteType)
	}

	if ftm.isArchived {
		newDir = filepath.Join(newDir, ".archive")
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
			fmt.Fprintf(sm.output, "WARNING: Collision: %s -> %s\n", filename, filepath.Base(newPath))
		}
	}

	if sm.options.DryRun {
		if sm.options.Verbose {
			fmt.Fprintf(sm.output, "Would migrate: %s -> %s\n", ftm.oldPath, newPath)
			if modified {
				fmt.Fprintf(sm.output, "  - Backfilled frontmatter fields\n")
			}
		}
		sm.report.MigratedFiles++
		return nil
	}

	// Ensure new directory exists (only in actual migration, not dry run)
	if err := os.MkdirAll(newDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write to new location
	if err := os.WriteFile(newPath, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	// Preserve timestamps
	if err := os.Chtimes(newPath, stat.ModTime(), stat.ModTime()); err != nil {
		// Non-fatal, just log
		if sm.options.Verbose {
			fmt.Fprintf(sm.output, "WARNING: Failed to preserve timestamps for %s\n", newPath)
		}
	}

	// Remove old file
	if err := os.Remove(ftm.oldPath); err != nil {
		return fmt.Errorf("failed to remove old file: %w", err)
	}

	sm.report.MigratedFiles++
	if modified {
		sm.report.IssuesFixed++
	}

	if sm.options.Verbose {
		fmt.Fprintf(sm.output, "* Migrated: %s -> %s\n", ftm.oldPath, newPath)
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

// getGlobalNotebookRoot returns the root directory for global notes
func (sm *StructuralMigration) getGlobalNotebookRoot() (string, error) {
	if sm.globalRoot == "" {
		return "", fmt.Errorf("global notebook root not configured")
	}
	return sm.globalRoot, nil
}

// isPlanDirectory checks if a directory is actually a plan directory (not just a note type directory)
func (sm *StructuralMigration) isPlanDirectory(path string) bool {
	dirName := filepath.Base(path)

	// Common note type names - if the directory matches one, it's NOT a plan
	commonNoteTypes := map[string]bool{
		"inbox":        true,
		"current":      true, // old name for inbox
		"llm":          true,
		"learn":        true,
		"daily":        true,
		"issues":       true,
		"architecture": true,
		"todos":        true,
		"quick":        true,
		"archive":      true,
		"prompts":      true,
		"blog":         true,
		"chats":        true,
	}

	if commonNoteTypes[dirName] {
		return false // It's a note type directory, not a plan
	}

	// Check for .grove-plan.yml file (definitive indicator)
	if _, err := os.Stat(filepath.Join(path, ".grove-plan.yml")); err == nil {
		return true
	}

	// Check for job files (pattern: NN-*.md like 01-job.md, 02-another.md)
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			// Job files typically follow pattern: NN-name.md or NNN-name.md
			name := entry.Name()
			if len(name) >= 3 && name[0] >= '0' && name[0] <= '9' && name[1] >= '0' && name[1] <= '9' && name[2] == '-' {
				return true
			}
		}
	}

	return false
}

// migratePlanDirectory migrates an entire plan directory
func (sm *StructuralMigration) migratePlanDirectory(planPath string, isArchived bool) error {
	sm.report.ProcessedFiles++

	planName := filepath.Base(planPath)
	workspaceName, _, err := sm.parseLegacyPathForPlan(planPath)
	if err != nil {
		return fmt.Errorf("could not parse path for plan %s: %w", planName, err)
	}

	// Determine destination path
	// Build the path directly using the new structure, similar to how regular notes are migrated
	var finalDestPath string
	if workspaceName == "global" {
		// Global plans go to {globalRoot}/plans/{planName}
		globalRoot, err := sm.getGlobalNotebookRoot()
		if err != nil {
			return fmt.Errorf("failed to get global notebook root: %w", err)
		}
		if isArchived {
			finalDestPath = filepath.Join(globalRoot, "plans", ".archive", planName)
		} else {
			finalDestPath = filepath.Join(globalRoot, "plans", planName)
		}
	} else {
		// Regular workspace plans go to {targetPath}/workspaces/{workspace}/plans/{planName}
		if isArchived {
			finalDestPath = filepath.Join(sm.targetPath, "workspaces", workspaceName, "plans", ".archive", planName)
		} else {
			finalDestPath = filepath.Join(sm.targetPath, "workspaces", workspaceName, "plans", planName)
		}
	}

	if sm.options.DryRun {
		if sm.options.Verbose {
			operation := "move"
			if sm.isCopyOnly {
				operation = "copy"
			}
			fmt.Fprintf(sm.output, "Would %s plan: %s -> %s\n", operation, planPath, finalDestPath)
		}
		sm.report.MigratedFiles++
		return nil
	}

	// Perform operation (copy or move)
	if err := os.MkdirAll(filepath.Dir(finalDestPath), 0755); err != nil {
		return fmt.Errorf("failed to create parent directory for destination: %w", err)
	}

	if sm.isCopyOnly {
		if err := fs.CopyDir(planPath, finalDestPath); err != nil {
			return fmt.Errorf("failed to copy plan directory: %w", err)
		}
	} else {
		if err := os.Rename(planPath, finalDestPath); err != nil {
			return fmt.Errorf("failed to move plan directory: %w", err)
		}
	}

	// Note: We do NOT update frontmatter for files inside plan directories.
	// Plan job files have their own schema (status, completed_at, template, etc.)
	// which is different from regular note frontmatter. Updating would corrupt them.

	if sm.options.Verbose {
		operation := "Moved"
		if sm.isCopyOnly {
			operation = "Copied"
		}
		fmt.Fprintf(sm.output, "* %s plan: %s -> %s\n", operation, planPath, finalDestPath)
	}
	sm.report.MigratedFiles++

	return nil
}

func (sm *StructuralMigration) parseLegacyPathForPlan(planPath string) (workspace, branch string, err error) {
	relPath, err := filepath.Rel(sm.sourcePath, planPath)
	if err != nil {
		return "", "", err
	}
	parts := strings.Split(filepath.ToSlash(relPath), "/")
	// Expected structure: repos/{workspace}/{branch}/(plans|archive)/{planName}
	if len(parts) >= 4 && parts[0] == "repos" {
		return parts[1], parts[2], nil
	}
	// Expected structure: global/(plans|archive)/{planName}
	if len(parts) >= 2 && parts[0] == "global" {
		return "global", "", nil
	}
	return "", "", fmt.Errorf("unrecognized plan path structure: %s", relPath)
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
