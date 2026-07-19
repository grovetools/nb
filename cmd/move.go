package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/grovetools/core/pkg/models"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/grovetools/nb/pkg/frontmatter"
	"github.com/grovetools/nb/pkg/migration"
	"github.com/grovetools/nb/pkg/service"
)

// emitNoteEvent routes every relocation performed by this command through the
// single note-mutation funnel. It is a package-level var (mirroring
// pkg/doctor's seam) so tests can capture what the move paths emit without
// standing up a daemon.
var emitNoteEvent = service.EmitNoteEvent

// emitMoveEvent reports a completed relocation to the note-event funnel so the
// daemon's index tracks the new location. PrevPath/PrevWorkspace/PrevNoteType
// are the rename-detection linchpin: without them the daemon sees a delete plus
// a create instead of a move. A --copy leaves the source in place, so it is a
// creation rather than a move, matching TransferNotes in pkg/service.
func emitMoveEvent(sourcePath, destPath string, isCopy bool) {
	ws, _, noteType := service.GetNoteMetadata(destPath)
	prevWs, _, prevType := service.GetNoteMetadata(sourcePath)

	eventType := models.NoteEventMoved
	if isCopy {
		eventType = models.NoteEventCreated
	}

	emitNoteEvent(models.NoteEvent{
		Event:         eventType,
		Workspace:     ws,
		NoteType:      noteType,
		Path:          destPath,
		PrevWorkspace: prevWs,
		PrevNoteType:  prevType,
		PrevPath:      sourcePath,
	})
}

func NewMoveCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	var (
		moveTargetWorkspace string
		moveTargetBranch    string
		moveTargetType      string
		moveApplyMigrate    bool
		moveDryRun          bool
		moveForce           bool
		moveCopy            bool
	)

	cmd := &cobra.Command{
		Use:   "move [file] [destination]",
		Short: "Move or copy notes to different locations",
		Long: `Move or copy notes to different locations within the note system.

By default, this command moves files (deletes the source after copying).
Use --copy to preserve the original file.

You can move notes between:
- Different note types (e.g., from 'current' to 'learn')
- Different branches (within the same repository)
- Different workspaces/repositories
- From outside the nb system into it

The destination can be specified as:
- A note type: "learn", "current", "llm", etc.
- A full path: "/path/to/destination/"
- Using flags: --workspace, --branch, --type

Examples:
  # Move current file to 'learn' type in same workspace
  nb move note.md learn

  # Move to different workspace
  nb move note.md --workspace other-project --type current

  # Move from outside nb system into current context
  nb move /tmp/external-note.md current

  # Move with migration (apply formatting/frontmatter fixes)
  nb move note.md learn --migrate

  # Preview what would happen
  nb move note.md learn --dry-run

  # Copy instead of move
  nb move note.md learn --copy`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			s := *svc

			sourcePath := args[0]

			// Determine destination from args and flags
			var destType, destWorkspace, destBranch string

			if len(args) == 2 {
				// Check if second arg is a note type
				validTypes := []string{"current", "llm", "learn", "quick", "people", "places", "projects", "archive", "inbox", "issues", "in_progress", "review", "completed"}
				dest := args[1]

				isNoteType := false
				for _, t := range validTypes {
					if dest == t {
						destType = dest
						isNoteType = true
						break
					}
				}

				if !isNoteType {
					// Treat as a full path
					return moveToPath(s, sourcePath, dest, moveDryRun, moveCopy)
				}
			}

			// Use flags to override or specify destination
			if moveTargetType != "" {
				destType = moveTargetType
			}
			if moveTargetWorkspace != "" {
				destWorkspace = moveTargetWorkspace
			}
			if moveTargetBranch != "" {
				destBranch = moveTargetBranch
			}

			// If no destination type specified, error
			if destType == "" {
				return fmt.Errorf("destination note type must be specified (e.g., 'current', 'learn', etc.)")
			}

			return moveNote(s, *workspaceOverride, sourcePath, destType, destWorkspace, destBranch,
				moveApplyMigrate, moveDryRun, moveForce, moveCopy)
		},
	}

	cmd.Flags().StringVar(&moveTargetWorkspace, "workspace", "", "Target workspace/repository")
	cmd.Flags().StringVar(&moveTargetBranch, "branch", "", "Target branch (for git repositories)")
	cmd.Flags().StringVarP(&moveTargetType, "type", "t", "", "Target note type (current, llm, learn, etc.)")
	cmd.Flags().BoolVar(&moveApplyMigrate, "migrate", true, "Apply nb migrate to standardize the note")
	cmd.Flags().BoolVar(&moveDryRun, "dry-run", false, "Preview changes without moving files")
	cmd.Flags().BoolVar(&moveForce, "force", false, "Overwrite existing files at destination")
	cmd.Flags().BoolVar(&moveCopy, "copy", false, "Copy instead of move (preserve original file)")

	return cmd
}

func moveNote(svc *service.Service, workspaceOverride, sourcePath, destType, destWorkspace, destBranch string,
	applyMigrate, dryRun, force, copy bool,
) error {
	// Make source path absolute
	absSource, err := filepath.Abs(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to resolve source path: %w", err)
	}

	// Check if source exists
	info, err := os.Stat(absSource)
	if err != nil {
		return fmt.Errorf("source file not found: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("source must be a file, not a directory")
	}

	// Get current context if destination workspace/branch not specified
	if destWorkspace == "" {
		ctx, err := svc.GetWorkspaceContext(workspaceOverride)
		if err != nil {
			return fmt.Errorf("could not determine destination workspace context: %w", err)
		}
		destWorkspace = ctx.NotebookContextWorkspace.Name
		if destBranch == "" {
			destBranch = ctx.Branch
		}
	}

	// Build destination path
	destPath, err := svc.BuildNotePath(destWorkspace, destBranch, destType, filepath.Base(absSource))
	if err != nil {
		return fmt.Errorf("failed to build destination path: %w", err)
	}

	if dryRun {
		if copy {
			fmt.Printf("Would copy:\n")
		} else {
			fmt.Printf("Would move:\n")
		}
		fmt.Printf("  From: %s\n", absSource)
		fmt.Printf("  To:   %s\n", destPath)
		fmt.Printf("  Workspace: %s\n", destWorkspace)
		if destBranch != "" {
			fmt.Printf("  Branch: %s\n", destBranch)
		}
		fmt.Printf("  Type: %s\n", destType)

		if applyMigrate {
			fmt.Printf("\nWould apply migration to standardize frontmatter and filename\n")
		}
		return nil
	}

	finalPath, err := relocateNote(absSource, destPath, applyMigrate, force, copy)
	if err != nil {
		return err
	}

	operation := "move"
	if copy {
		operation = "copy"
		fmt.Printf("Copied note successfully:\n")
	} else {
		fmt.Printf("Moved note successfully:\n")
	}
	fmt.Printf("  From: %s\n", absSource)
	fmt.Printf("  To:   %s\n", finalPath)

	svc.Logger.WithFields(logrus.Fields{
		"operation":   operation,
		"source_path": absSource,
		"dest_path":   finalPath,
		"migrated":    applyMigrate,
	}).Info("Move/copy operation completed")

	return nil
}

// relocateNote performs the filesystem half of `nb move` once the destination
// path is known, applies migration when asked, and reports the result through
// the note-event funnel. It returns the note's final path, which differs from
// destPath when migration renamed the file.
//
// Every exit that actually relocated the note emits: this is the path the plan
// lifecycle hooks drive (flow's orchestration.MoveNote shells out to
// `nb move <path> <group> --force`), and skipping the emit left the daemon's
// note index stale on exactly the inbox -> in_progress -> review -> completed
// transitions the lifecycle is made of.
func relocateNote(absSource, destPath string, applyMigrate, force, copy bool) (string, error) {
	// Check if destination exists
	if _, err := os.Stat(destPath); err == nil && !force {
		return "", fmt.Errorf("destination file already exists: %s (use --force to overwrite)", destPath)
	}

	// Create destination directory if needed
	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create destination directory: %w", err)
	}

	isInNbSystem := strings.Contains(absSource, "/nb/")

	switch {
	case copy:
		// Always copy when --copy flag is used
		if err := copyFile(absSource, destPath); err != nil {
			return "", fmt.Errorf("failed to copy file: %w", err)
		}
	case isInNbSystem && !applyMigrate:
		// Simple rename for moves within nb system without migration
		if err := os.Rename(absSource, destPath); err != nil {
			// If rename fails (e.g., cross-device), fall back to copy and delete.
			// This used to return early, silently skipping both the event emit
			// and the caller's "To:" output that flow parses.
			if err := copyAndDelete(absSource, destPath); err != nil {
				return "", err
			}
		}
	default:
		// Copy the file first
		if err := copyFile(absSource, destPath); err != nil {
			return "", fmt.Errorf("failed to copy file: %w", err)
		}

		// Delete source file after successful copy
		if err := os.Remove(absSource); err != nil {
			// Non-fatal - warn but continue
			fmt.Fprintf(os.Stderr, "Warning: failed to remove source file: %v\n", err)
		}
	}

	// Apply migration if requested
	finalPath := destPath
	if applyMigrate {
		migrated, err := applyMigration(destPath)
		if err != nil {
			return "", fmt.Errorf("failed to apply migration: %w", err)
		}
		finalPath = migrated
	}

	emitMoveEvent(absSource, finalPath, copy)

	return finalPath, nil
}

func moveToPath(svc *service.Service, sourcePath, destPath string, dryRun, copy bool) error {
	// Implementation for moving to a specific path
	// This is a simplified version - you might want to add more validation
	absSource, err := filepath.Abs(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to resolve source path: %w", err)
	}

	absDest, err := filepath.Abs(destPath)
	if err != nil {
		return fmt.Errorf("failed to resolve destination path: %w", err)
	}

	// If destination is a directory, use source filename
	info, err := os.Stat(absDest)
	if err == nil && info.IsDir() {
		absDest = filepath.Join(absDest, filepath.Base(absSource))
	}

	if dryRun {
		fmt.Printf("Would move:\n")
		fmt.Printf("  From: %s\n", absSource)
		fmt.Printf("  To:   %s\n", absDest)
		return nil
	}

	// Create destination directory if needed
	destDir := filepath.Dir(absDest)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Try rename first
	if err := os.Rename(absSource, absDest); err != nil {
		// Fall back to copy and delete (e.g. cross-device)
		if err := copyAndDelete(absSource, absDest); err != nil {
			return err
		}
	}

	// Same funnel as the note-type path above: an explicit destination path is
	// still a relocation the daemon's index has to learn about. This path always
	// relocates (it ignores its copy parameter), so it is always a move.
	emitMoveEvent(absSource, absDest, false)

	fmt.Printf("Moved successfully: %s -> %s\n", absSource, absDest)
	return nil
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

	_, err = destination.ReadFrom(source)
	if err != nil {
		return err
	}

	// Copy file permissions
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
		// Non-fatal - warn but continue
		fmt.Fprintf(os.Stderr, "Warning: failed to remove source file: %v\n", err)
	}

	return nil
}

func applyMigration(filePath string) (string, error) {
	// Use the centralized migration package to check if file needs migration
	basePath := filepath.Join(os.Getenv("HOME"), "Documents", "nb")
	issues, err := migration.AnalyzeFile(filePath, basePath)
	if err != nil {
		return filePath, fmt.Errorf("analyze file: %w", err)
	}

	if len(issues) == 0 {
		// No migration needed
		return filePath, nil
	}

	// Capture the deterministic handles on the file's post-migration identity
	// BEFORE migrating, so the new location never has to be rediscovered by
	// guesswork:
	//   - the standardized filename the analyzer computed (the rename target the
	//     migrator will use; derived from id date + title slug, so re-analysing
	//     the same file yields the same answer), and
	//   - the note's stable frontmatter id, which survives moves and retitles.
	expectedName := ""
	for _, issue := range issues {
		if issue.Type == "non_standard_filename" {
			if name, ok := issue.Expected.(string); ok {
				expectedName = name
			}
		}
	}
	// Only a pre-existing id is a reliable handle. When the note has no valid id
	// the migrator mints one containing time.Now() to the second, which we cannot
	// predict, so we fall back to the filename handle alone.
	stableID := noteIDAt(filePath)

	fmt.Printf("Applying migration to standardize note format...\n")

	// Apply migration using the centralized migration package
	options := migration.MigrationOptions{
		DryRun:   false,
		Verbose:  true,
		NoBackup: true, // Since we're moving, we don't need a backup
	}

	err = migration.MigrateFile(filePath, basePath, options, os.Stdout)
	if err != nil {
		return filePath, fmt.Errorf("migrate file: %w", err)
	}

	// The file might have been renamed during migration. If the original path is
	// still there, nothing moved and there is nothing to resolve.
	if _, err := os.Stat(filePath); err == nil {
		return filePath, nil
	}

	return resolveMigratedPath(filePath, expectedName, stableID), nil
}

// resolveMigratedPath locates a note after migration renamed it, deterministically.
//
// It deliberately does NOT fall back to "the most recently modified .md in the
// directory": that guess silently returns some other note whenever anything else
// writes a .md in the same window, and it is not even sound here — the migrator
// restores the original mtime with os.Chtimes, so the migrated file is usually
// NOT the newest one.
//
// Resolution order, strongest first:
//  1. The standardized filename the analyzer computed, confirmed by frontmatter
//     id when we have one (the migrator appends a -N suffix on collision, so a
//     name match alone can point at the colliding file).
//  2. The note's stable frontmatter id, scanned for across the directory. This
//     is authoritative: ids survive both moves and retitles.
//
// If neither resolves, the original path is returned unchanged rather than a
// guess, leaving the caller with the same value it passed in.
func resolveMigratedPath(origPath, expectedName, stableID string) string {
	dir := filepath.Dir(origPath)

	if expectedName != "" {
		candidate := filepath.Join(dir, expectedName)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			if stableID == "" || noteIDAt(candidate) == stableID {
				return candidate
			}
		}
	}

	if stableID != "" {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return origPath
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			candidate := filepath.Join(dir, entry.Name())
			if noteIDAt(candidate) == stableID {
				return candidate
			}
		}
	}

	return origPath
}

// noteIDAt returns the stable frontmatter id of the note at path, or "" when the
// file is unreadable, has no frontmatter, or carries no id.
func noteIDAt(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	fm, _, err := frontmatter.Parse(string(content))
	if err != nil || fm == nil {
		return ""
	}
	return fm.ID
}
