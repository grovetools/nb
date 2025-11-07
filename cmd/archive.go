package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/mattsolo1/grove-notebook/cmd/config"
	"github.com/mattsolo1/grove-notebook/pkg/models"
)

func NewArchiveCmd() *cobra.Command {
	var (
		olderThan    int
		dryRun       bool
		forceArchive bool
	)

	cmd := &cobra.Command{
		Use:   "archive [files...]",
		Short: "Archive notes",
		Long: `Move notes to the archive directory.

Examples:
  nb archive note1.md note2.md     # Archive specific files
  nb archive --older-than 30       # Archive notes older than 30 days
  nb archive --dry-run             # Show what would be archived`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Initialize config and service
			config.InitConfig()
			svc, err := config.InitService()
			if err != nil {
				return err
			}
			defer svc.Close()

			// Get workspace context
			ctx, err := svc.GetWorkspaceContext(config.WorkspaceOverride)
			if err != nil {
				return fmt.Errorf("get workspace context: %w", err)
			}

			var filesToArchive []string

			if len(args) > 0 {
				// Archive specific files - need to resolve to full paths

				for _, arg := range args {
					// If it's already an absolute path, use it
					if filepath.IsAbs(arg) {
						filesToArchive = append(filesToArchive, arg)
						continue
					}

					// Otherwise, search the entire workspace for the file
					workspaceRoot := ctx.CurrentWorkspace.Path

					_ = filepath.Walk(workspaceRoot, func(path string, info os.FileInfo, err error) error {
						if err != nil {
							return nil
						}

						// Skip archive directory
						if strings.Contains(path, "/archive/") {
							return nil
						}

						if !info.IsDir() && filepath.Base(path) == arg {
							filesToArchive = append(filesToArchive, path)
						}
						return nil
					})

					if len(filesToArchive) == 0 {
						return fmt.Errorf("file not found in workspace: %s", arg)
					}
				}
			} else if olderThan > 0 {
				// Find old notes to archive across all note types
				noteTypes := []models.NoteType{
					"current",
					"llm",
					"learn",
					"daily",
					"issues",
					"architecture",
					"todos",
					"blog",
					"prompts",
				}

				cutoff := time.Now().AddDate(0, 0, -olderThan)

				for _, noteType := range noteTypes {
					notes, err := svc.ListNotes(ctx, noteType)
					if err != nil {
						continue // Skip if directory doesn't exist
					}

					for _, note := range notes {
						if note.ModifiedAt.Before(cutoff) {
							// Only skip notes with active todos if they're recent
							if note.HasTodos && note.ModifiedAt.After(cutoff.AddDate(0, 0, -7)) {
								continue
							}
							filesToArchive = append(filesToArchive, note.Path)
						}
					}
				}
			} else {
				return fmt.Errorf("specify files to archive or use --older-than")
			}

			if len(filesToArchive) == 0 {
				fmt.Println("No files to archive")
				return nil
			}

			if dryRun {
				fmt.Println("Would archive:")
				for _, file := range filesToArchive {
					fmt.Printf("  %s\n", file)
				}
				return nil
			}

			// Confirm unless --force is used
			if !forceArchive {
				fmt.Printf("Archive %d files? [y/N] ", len(filesToArchive))
				var response string
				_, _ = fmt.Scanln(&response)

				if strings.ToLower(response) != "y" {
					fmt.Println("Cancelled")
					return nil
				}
			}

			// Archive the files
			if err := svc.ArchiveNotes(ctx, filesToArchive); err != nil {
				return err
			}

			fmt.Printf("Archived %d files\n", len(filesToArchive))
			return nil
		},
	}

	cmd.Flags().IntVar(&olderThan, "older-than", 0, "Archive notes older than N days")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be archived without doing it")
	cmd.Flags().BoolVar(&forceArchive, "force", false, "Skip confirmation prompt")

	// Add global flags
	config.AddGlobalFlags(cmd)

	return cmd
}
