package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mattsolo1/grove-core/util/pathutil"
	"github.com/mattsolo1/grove-notebook/pkg/migration"
	"github.com/mattsolo1/grove-notebook/pkg/service"
)

const (
	globalStr   = "global"
	untitledStr = "untitled"
)

func NewMigrateCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	var (
		migrateDryRun        bool
		migrateForce         bool
		migrateRecursive     bool
		fixTitles            bool
		fixDates             bool
		fixTags              bool
		fixIDs               bool
		fixFilenames         bool
		preserveTimestamps   bool
		migrateAll           bool
		migrateGlobal        bool
		migrateWorkspace     string
		migrateBranch        string
		migrateType          string
		allWorkspaces        bool
		migrateVerbose       bool
		migrateShowReport    bool
		migrateNoBackup      bool
		migrateStructure     bool
		migrateTarget        string
		migrateYes           bool
		renameCurrentToInbox bool
		ensureTypeInTags     bool
		migrateNotebook      string
		workspaceRenames     string
		sourceNotebook       string
		targetNotebook       string
	)

	cmd := &cobra.Command{
		Use:   "migrate [paths...]",
		Short: "Migrate and standardize notes",
		Long: `Migrate notes to standardized format with proper frontmatter.
	
By default, operates on the current workspace context.

Examples:
  nb migrate --dry-run                    # Preview changes in current context
  nb migrate --fix-titles                 # Fix titles in current context
  nb migrate --global --all               # Migrate all global notes
  nb migrate --workspace myproject --all  # Migrate entire workspace`,
		RunE: func(cmd *cobra.Command, args []string) error {
			s := *svc

			// Handle structural migration separately
			if migrateStructure {
				return runStructuralMigration(s, migrateDryRun, migrateVerbose, migrateShowReport, migrateTarget, migrateYes, migrateNotebook)
			}

			// Handle type migration
			if renameCurrentToInbox {
				// Determine which notebook to use
				notebookName := migrateNotebook
				if notebookName == "" && s.CoreConfig != nil && s.CoreConfig.Notebooks != nil && s.CoreConfig.Notebooks.Rules != nil {
					notebookName = s.CoreConfig.Notebooks.Rules.Default
				}
				if notebookName == "" {
					notebookName = "nb" // Final fallback
				}

				// Get the notebook root path
				var notebookRoot string
				if s.CoreConfig != nil && s.CoreConfig.Notebooks != nil && s.CoreConfig.Notebooks.Definitions != nil {
					if notebook, exists := s.CoreConfig.Notebooks.Definitions[notebookName]; exists && notebook != nil {
						if notebook.RootDir != "" {
							expandedPath, err := pathutil.Expand(notebook.RootDir)
							if err != nil {
								return fmt.Errorf("failed to expand notebook root_dir '%s': %w", notebook.RootDir, err)
							}
							notebookRoot = expandedPath
						}
					}
				}

				// Fallback to default notebook path if not configured
				if notebookRoot == "" {
					notebookRoot = filepath.Join(os.Getenv("HOME"), "Documents", "nb")
				}

				report, err := migration.RenameCurrentToInbox(notebookRoot, migration.MigrationOptions{
					DryRun:     migrateDryRun,
					Verbose:    migrateVerbose,
					ShowReport: migrateShowReport,
				}, os.Stdout)
				if err != nil {
					return fmt.Errorf("failed to rename 'current' to 'inbox': %w", err)
				}
				if migrateShowReport {
					printMigrationReport(report, migrateDryRun)
				}
				return nil
			}

			// Handle ensure-type-in-tags migration
			if ensureTypeInTags {
				// Determine which notebook to use
				notebookName := migrateNotebook
				if notebookName == "" && s.CoreConfig != nil && s.CoreConfig.Notebooks != nil && s.CoreConfig.Notebooks.Rules != nil {
					notebookName = s.CoreConfig.Notebooks.Rules.Default
				}
				if notebookName == "" {
					notebookName = "nb" // Final fallback
				}

				// Get the notebook root path
				var notebookRoot string
				if s.CoreConfig != nil && s.CoreConfig.Notebooks != nil && s.CoreConfig.Notebooks.Definitions != nil {
					if notebook, exists := s.CoreConfig.Notebooks.Definitions[notebookName]; exists && notebook != nil {
						if notebook.RootDir != "" {
							expandedPath, err := pathutil.Expand(notebook.RootDir)
							if err != nil {
								return fmt.Errorf("failed to expand notebook root_dir '%s': %w", notebook.RootDir, err)
							}
							notebookRoot = expandedPath
						}
					}
				}

				// Fallback to default notebook path if not configured
				if notebookRoot == "" {
					notebookRoot = filepath.Join(os.Getenv("HOME"), "Documents", "nb")
				}

				report, err := migration.EnsureTypeInTags(notebookRoot, migration.MigrationOptions{
					DryRun:     migrateDryRun,
					Verbose:    migrateVerbose,
					ShowReport: migrateShowReport,
				}, os.Stdout)
				if err != nil {
					return fmt.Errorf("failed to ensure type in tags: %w", err)
				}
				if migrateShowReport {
					printMigrationReport(report, migrateDryRun)
				}
				return nil
			}

			// Handle workspace renames migration
			if workspaceRenames != "" {
				// Parse the renames (format: "old1=new1,old2=new2")
				renames := make(map[string]string)
				for _, pair := range strings.Split(workspaceRenames, ",") {
					parts := strings.SplitN(strings.TrimSpace(pair), "=", 2)
					if len(parts) != 2 {
						return fmt.Errorf("invalid rename format '%s', expected 'old=new'", pair)
					}
					renames[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
				}

				if len(renames) == 0 {
					return fmt.Errorf("no valid renames specified")
				}

				// Get source notebook root
				var sourceRoot string
				srcNotebookName := sourceNotebook
				if srcNotebookName == "" && s.CoreConfig != nil && s.CoreConfig.Notebooks != nil && s.CoreConfig.Notebooks.Rules != nil {
					srcNotebookName = s.CoreConfig.Notebooks.Rules.Default
				}
				if srcNotebookName == "" {
					srcNotebookName = "nb"
				}
				if s.CoreConfig != nil && s.CoreConfig.Notebooks != nil && s.CoreConfig.Notebooks.Definitions != nil {
					if notebook, exists := s.CoreConfig.Notebooks.Definitions[srcNotebookName]; exists && notebook != nil {
						if notebook.RootDir != "" {
							expandedPath, err := pathutil.Expand(notebook.RootDir)
							if err != nil {
								return fmt.Errorf("failed to expand source notebook root_dir '%s': %w", notebook.RootDir, err)
							}
							sourceRoot = expandedPath
						}
					}
				}
				if sourceRoot == "" {
					sourceRoot = filepath.Join(os.Getenv("HOME"), "notebooks", srcNotebookName)
				}

				// Get target notebook root
				var targetRoot string
				if targetNotebook != "" {
					if s.CoreConfig != nil && s.CoreConfig.Notebooks != nil && s.CoreConfig.Notebooks.Definitions != nil {
						if notebook, exists := s.CoreConfig.Notebooks.Definitions[targetNotebook]; exists && notebook != nil {
							if notebook.RootDir != "" {
								expandedPath, err := pathutil.Expand(notebook.RootDir)
								if err != nil {
									return fmt.Errorf("failed to expand target notebook root_dir '%s': %w", notebook.RootDir, err)
								}
								targetRoot = expandedPath
							}
						}
					}
					if targetRoot == "" {
						targetRoot = filepath.Join(os.Getenv("HOME"), "notebooks", targetNotebook)
					}
				} else {
					// If no target specified, use source (in-place migration)
					targetRoot = sourceRoot
				}

				fmt.Printf("Workspace Migration\n")
				fmt.Printf("===================\n")
				fmt.Printf("Source notebook: %s\n", sourceRoot)
				fmt.Printf("Target notebook: %s\n", targetRoot)
				fmt.Printf("Renames:\n")
				for old, newName := range renames {
					fmt.Printf("  %s -> %s\n", old, newName)
				}
				fmt.Println()

				if !migrateDryRun && !migrateYes {
					fmt.Print("Continue with workspace migration? [y/N] ")
					var response string
					_, _ = fmt.Scanln(&response)
					if strings.ToLower(response) != "y" {
						fmt.Println("Cancelled")
						return nil
					}
				}

				report, err := migration.MigrateWorkspaces(sourceRoot, targetRoot, renames, migration.MigrationOptions{
					DryRun:     migrateDryRun,
					Verbose:    migrateVerbose,
					ShowReport: migrateShowReport,
				}, os.Stdout)
				if err != nil {
					return fmt.Errorf("workspace migration failed: %w", err)
				}
				if migrateShowReport {
					printMigrationReport(report, migrateDryRun)
				}
				return nil
			}

			if migrateAll {
				fixTitles = true
				fixDates = true
				fixTags = true
				fixIDs = true
				fixFilenames = true
			}

			if !fixTitles && !fixDates && !fixTags && !fixIDs && !fixFilenames {
				return fmt.Errorf("specify at least one fix option or use --all")
			}

			if allWorkspaces && !migrateForce {
				fmt.Print("This will migrate ALL notes in your entire notebook. Continue? [y/N] ")
				var response string
				_, _ = fmt.Scanln(&response)
				if strings.ToLower(response) != "y" {
					fmt.Println("Cancelled")
					return nil
				}
			}

			basePath := filepath.Join(os.Getenv("HOME"), "Documents", "nb")

			var scope migration.MigrationScope
			switch {
			case allWorkspaces:
				scope.All = true
			case migrateGlobal:
				scope.Global = true
			case migrateWorkspace != "":
				scope.Workspace = migrateWorkspace
			default:
				context, err := s.GetWorkspaceContext(*workspaceOverride)
				if err != nil {
					return fmt.Errorf("get context: %w", err)
				}
				scope.Context = context.NotebookContextWorkspace.Name
			}

			options := migration.MigrationOptions{
				Scope:      scope,
				DryRun:     migrateDryRun,
				Verbose:    migrateVerbose,
				ShowReport: migrateShowReport,
				NoBackup:   migrateNoBackup,
			}

			report, err := migration.Migrate(basePath, options, os.Stdout, s.Logger)
			if err != nil {
				return fmt.Errorf("migration failed: %w", err)
			}

			if migrateShowReport {
				printMigrationReport(report, migrateDryRun)
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&migrateStructure, "structure", false, "Migrate from old repos/{workspace}/{branch} structure to new notebooks structure")
	cmd.Flags().BoolVar(&renameCurrentToInbox, "rename-current-to-inbox", false, "Rename 'current' note type directories to 'inbox' and update notes")
	cmd.Flags().BoolVar(&ensureTypeInTags, "ensure-type-in-tags", false, "Ensure all notes have their type in the tags array")
	cmd.Flags().StringVar(&migrateNotebook, "notebook", "", "Specify which notebook to migrate (default: uses default notebook from config)")
	cmd.Flags().StringVar(&workspaceRenames, "workspaces", "", "Rename workspaces: 'old1=new1,old2=new2' (copies all files, updates frontmatter)")
	cmd.Flags().StringVar(&sourceNotebook, "source-notebook", "", "Source notebook for workspace migration (default: default notebook)")
	cmd.Flags().StringVar(&targetNotebook, "target-notebook", "", "Target notebook for workspace migration (default: same as source)")
	cmd.Flags().StringVar(&migrateTarget, "target", "", "Target directory for migration (default: uses notebook root_dir from config)")
	cmd.Flags().BoolVarP(&migrateYes, "yes", "y", false, "Skip confirmation prompts")
	cmd.Flags().BoolVar(&migrateDryRun, "dry-run", false, "Show what would be changed without modifying")
	cmd.Flags().BoolVar(&migrateForce, "force", false, "Overwrite existing frontmatter")
	cmd.Flags().BoolVar(&migrateRecursive, "recursive", true, "Process directories recursively")
	cmd.Flags().BoolVar(&fixTitles, "fix-titles", false, "Extract titles from content if missing")
	cmd.Flags().BoolVar(&fixDates, "fix-dates", false, "Use file mtime if no date in frontmatter")
	cmd.Flags().BoolVar(&fixTags, "fix-tags", false, "Generate tags from path/repo/branch")
	cmd.Flags().BoolVar(&fixIDs, "fix-ids", false, "Generate missing IDs")
	cmd.Flags().BoolVar(&fixFilenames, "fix-filenames", false, "Standardize filenames to YYYYMMDD-title.md format")
	cmd.Flags().BoolVar(&preserveTimestamps, "preserve-timestamps", true, "Preserve original file modification times")
	cmd.Flags().BoolVar(&migrateAll, "all", false, "Apply all fixes")
	cmd.Flags().BoolVar(&migrateGlobal, globalStr, false, "Process global notes instead of current context")
	cmd.Flags().StringVar(&migrateWorkspace, "workspace", "", "Process specific workspace")
	cmd.Flags().StringVar(&migrateBranch, "branch", "", "Process specific branch")
	cmd.Flags().StringVar(&migrateType, "type", "", "Process only specific note type")
	cmd.Flags().BoolVar(&allWorkspaces, "all-workspaces", false, "Process all workspaces (requires confirmation)")
	cmd.Flags().BoolVar(&migrateVerbose, "verbose", false, "Show detailed output")
	cmd.Flags().BoolVar(&migrateShowReport, "report", true, "Show migration report")
	cmd.Flags().BoolVar(&migrateNoBackup, "no-backup", false, "Don't create backup files")

	return cmd
}

func printMigrationReport(report *migration.MigrationReport, dryRun bool) {
	fmt.Printf("\nMigration Report\n")
	fmt.Printf("================\n")
	fmt.Printf("Total files:     %d\n", report.TotalFiles)
	fmt.Printf("Processed:       %d\n", report.ProcessedFiles)
	fmt.Printf("Migrated:        %d\n", report.MigratedFiles)
	fmt.Printf("Skipped:         %d\n", report.SkippedFiles)
	fmt.Printf("Failed:          %d\n", report.FailedFiles)

	if report.MigratedFiles > 0 {
		fmt.Printf("Issues fixed:    %d\n", report.IssuesFixed)
	}

	if report.CreatedFiles > 0 || report.DeletedFiles > 0 {
		fmt.Printf("Files renamed:   %d\n", report.CreatedFiles)
	}

	fmt.Printf("Duration:        %s\n", report.Duration())

	if len(report.ProcessingErrors) > 0 {
		fmt.Printf("\nErrors:\n")
		for file, err := range report.ProcessingErrors {
			fmt.Printf("  %s: %v\n", file, err)
		}
	}

	if dryRun {
		fmt.Println("\nDry run complete. No files were modified.")
	}
}

func runStructuralMigration(svc *service.Service, dryRun, verbose, showReport bool, targetDir string, skipConfirm bool, notebookFlag string) error {
	// If --target is used, it's a copy-only operation (non-destructive)
	isCopyOnly := targetDir != ""

	// Determine which notebook to use
	notebookName := notebookFlag
	if notebookName == "" && svc.CoreConfig != nil && svc.CoreConfig.Notebooks != nil && svc.CoreConfig.Notebooks.Rules != nil {
		notebookName = svc.CoreConfig.Notebooks.Rules.Default
	}
	if notebookName == "" {
		notebookName = "nb" // Final fallback
	}

	// Get the source path (where old files are)
	var sourcePath string
	if svc.CoreConfig != nil && svc.CoreConfig.Notebooks != nil && svc.CoreConfig.Notebooks.Definitions != nil {
		if notebook, exists := svc.CoreConfig.Notebooks.Definitions[notebookName]; exists && notebook != nil {
			if notebook.RootDir != "" {
				expandedPath, err := pathutil.Expand(notebook.RootDir)
				if err != nil {
					return fmt.Errorf("failed to expand notebook root_dir '%s': %w", notebook.RootDir, err)
				}
				sourcePath = expandedPath
			}
		}
	}

	// Fallback to default source path if not configured
	if sourcePath == "" {
		sourcePath = filepath.Join(os.Getenv("HOME"), "Documents", "nb")
		fmt.Printf("Warning: No notebook '%s' configured, using fallback source path: %s\n", notebookName, sourcePath)
	}

	// Get the target path (where to migrate to)
	var targetPath string
	if targetDir != "" {
		// --target flag overrides config
		// Append notebook name to create structure like ~/notebooks/nb/
		expandedPath, err := pathutil.Expand(targetDir)
		if err != nil {
			return fmt.Errorf("failed to expand target path '%s': %w", targetDir, err)
		}
		targetPath = filepath.Join(expandedPath, notebookName)
		if verbose {
			fmt.Printf("Migrating from: %s\n", sourcePath)
			fmt.Printf("Migrating to: %s\n", targetPath)
		}
	} else {
		// If no --target, migrate within the same directory (source == target)
		targetPath = sourcePath
		if verbose {
			fmt.Printf("Migrating within: %s\n", sourcePath)
		}
	}

	// Get the global notebook root_dir from config
	var globalRoot string
	if svc.CoreConfig != nil && svc.CoreConfig.Notebooks != nil &&
		svc.CoreConfig.Notebooks.Rules != nil &&
		svc.CoreConfig.Notebooks.Rules.Global != nil {

		if svc.CoreConfig.Notebooks.Rules.Global.RootDir != "" {
			expandedPath, err := pathutil.Expand(svc.CoreConfig.Notebooks.Rules.Global.RootDir)
			if err != nil {
				return fmt.Errorf("failed to expand global notebook root_dir '%s': %w",
					svc.CoreConfig.Notebooks.Rules.Global.RootDir, err)
			}
			globalRoot = expandedPath
		}
	}

	// Fallback to default global path if not configured
	if globalRoot == "" {
		globalRoot = filepath.Join(os.Getenv("HOME"), ".grove", "notebooks", "global")
		if verbose {
			fmt.Printf("Using default global notebook path: %s\n", globalRoot)
		}
	}

	options := migration.MigrationOptions{
		DryRun:     dryRun,
		Verbose:    verbose,
		ShowReport: showReport,
	}

	// Get workspace provider and locator from service
	provider := svc.GetWorkspaceProvider()
	locator := svc.GetNotebookLocator()

	sm := migration.NewStructuralMigration(sourcePath, targetPath, globalRoot, locator, provider, options, isCopyOnly, os.Stdout)

	// Check if user has old templates that will conflict
	hasOldTemplates := false
	if svc.CoreConfig != nil && svc.CoreConfig.Notebooks != nil &&
		svc.CoreConfig.Notebooks.Definitions != nil {
		if nb, exists := svc.CoreConfig.Notebooks.Definitions[notebookName]; exists && nb != nil {
			if strings.Contains(nb.NotesPathTemplate, "repos/") ||
				strings.Contains(nb.PlansPathTemplate, "repos/") ||
				strings.Contains(nb.ChatsPathTemplate, "repos/") {
				hasOldTemplates = true
			}
		}
	}

	if hasOldTemplates && !skipConfirm {
		fmt.Println("WARNING:  WARNING: Your grove.yml contains old path templates that will NOT work after migration!")
		fmt.Println("\n   Current templates point to: repos/{workspace}/main/{noteType}")
		fmt.Println("   After migration, files will be at: workspaces/{workspace}/{noteType}")
		fmt.Println("\n   You MUST update your grove.yml after migration by removing these lines:")
		fmt.Println("     - chats_path_template")
		fmt.Println("     - notes_path_template")
		fmt.Println("     - plans_path_template")
		fmt.Println("\n   The new defaults will then apply automatically.")
		fmt.Print("\nDo you understand and will update your config? [y/N] ")
		var response string
		_, _ = fmt.Scanln(&response)
		if strings.ToLower(response) != "y" {
			fmt.Println("Cancelled - please update your config first or acknowledge the required changes")
			return nil
		}
	} else if hasOldTemplates && skipConfirm {
		fmt.Println("WARNING:  WARNING: Old path templates detected. Remember to update grove.yml after migration.")
	}

	if !isCopyOnly && !dryRun && !skipConfirm {
		fmt.Println("\nWARNING:  WARNING: This will migrate notes from the old `repos/` and `global/`")
		fmt.Println("   structures to the new `workspaces/` structure. This is a destructive")
		fmt.Println("   operation. Files will be moved, and upon successful completion, the")
		fmt.Println("   old `repos/` and `global/` directories will be REMOVED.")
		fmt.Println("\n   It is strongly recommended to back up your notebook directory before proceeding.")
		fmt.Print("\nContinue with structural migration? [y/N] ")
		var response string
		_, _ = fmt.Scanln(&response)
		if strings.ToLower(response) != "y" {
			fmt.Println("Cancelled")
			return nil
		}
	} else if !isCopyOnly && !dryRun && skipConfirm {
		fmt.Println("WARNING:  WARNING: Running structural migration (confirmations skipped with -y)")
	}

	if err := sm.MigrateStructure(); err != nil {
		return fmt.Errorf("structural migration failed: %w", err)
	}

	report := sm.GetReport()
	if showReport {
		printMigrationReport(report, dryRun)
	}

	// Remind about config update if needed
	if !dryRun && hasOldTemplates && report.MigratedFiles > 0 {
		fmt.Println("\n" + strings.Repeat("=", 80))
		fmt.Println("WARNING:  IMPORTANT: Update your grove.yml config NOW!")
		fmt.Println(strings.Repeat("=", 80))
		fmt.Println("\nEdit ~/.config/grove/grove.yml and remove these lines from the 'nb' notebook:")
		fmt.Println("  - chats_path_template: repos/{{ .Workspace.Name }}/main/current")
		fmt.Println("  - notes_path_template: repos/{{ .Workspace.Name }}/main/{{ .NoteType }}")
		fmt.Println("  - plans_path_template: repos/{{ .Workspace.Name }}/main/plans")
		fmt.Println("\nAlso update the root_dir to your new location:")
		fmt.Println("  root_dir: ~/notebooks/nb")
		fmt.Println("\nWithout these changes, nb will not find your migrated notes!")
		fmt.Println(strings.Repeat("=", 80))
	}

	return nil
}