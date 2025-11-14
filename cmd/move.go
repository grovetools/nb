package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mattsolo1/grove-notebook/pkg/migration"
	"github.com/mattsolo1/grove-notebook/pkg/service"
)

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
	applyMigrate, dryRun, force, copy bool) error {
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

	// Check if destination exists
	if _, err := os.Stat(destPath); err == nil && !force {
		return fmt.Errorf("destination file already exists: %s (use --force to overwrite)", destPath)
	}

	// Create destination directory if needed
	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	isInNbSystem := strings.Contains(absSource, "/nb/")

	if copy {
		// Always copy when --copy flag is used
		if err := copyFile(absSource, destPath); err != nil {
			return fmt.Errorf("failed to copy file: %w", err)
		}
	} else if isInNbSystem && !applyMigrate {
		// Simple rename for moves within nb system without migration
		if err := os.Rename(absSource, destPath); err != nil {
			// If rename fails (e.g., cross-device), fall back to copy and delete
			return copyAndDelete(absSource, destPath)
		}
	} else {
		// Copy the file first
		if err := copyFile(absSource, destPath); err != nil {
			return fmt.Errorf("failed to copy file: %w", err)
		}

		// Delete source file after successful copy (unless --copy flag is used)
		if !copy {
			if err := os.Remove(absSource); err != nil {
				// Non-fatal - warn but continue
				fmt.Fprintf(os.Stderr, "Warning: failed to remove source file: %v\n", err)
			}
		}
	}

	// Apply migration if requested
	finalPath := destPath
	if applyMigrate {
		var err error
		finalPath, err = applyMigration(svc, destPath)
		if err != nil {
			return fmt.Errorf("failed to apply migration: %w", err)
		}
	}

	if copy {
		fmt.Printf("Copied note successfully:\n")
	} else {
		fmt.Printf("Moved note successfully:\n")
	}
	fmt.Printf("  From: %s\n", absSource)
	fmt.Printf("  To:   %s\n", finalPath)

	return nil
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
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Try rename first
	if err := os.Rename(absSource, absDest); err != nil {
		// Fall back to copy and delete
		return copyAndDelete(absSource, absDest)
	}

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

func applyMigration(svc *service.Service, filePath string) (string, error) {
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

	// The file might have been renamed during migration
	// Check if the original file still exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// File was renamed, find the new path
		dir := filepath.Dir(filePath)
		entries, err := os.ReadDir(dir)
		if err != nil {
			return filePath, fmt.Errorf("read directory: %w", err)
		}

		// Find the most recently modified .md file (likely our migrated file)
		var newestFile string
		var newestTime int64
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".md") {
				info, err := entry.Info()
				if err == nil && info.ModTime().Unix() > newestTime {
					newestTime = info.ModTime().Unix()
					newestFile = filepath.Join(dir, entry.Name())
				}
			}
		}

		if newestFile != "" {
			return newestFile, nil
		}
	}

	return filePath, nil
}
