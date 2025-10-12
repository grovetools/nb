package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mattsolo1/grove-notebook/cmd/config"
	"github.com/mattsolo1/grove-notebook/pkg/migration"
)

const (
	globalStr   = "global"
	untitledStr = "untitled"
)

func NewMigrateCmd() *cobra.Command {
	var (
		migrateDryRun      bool
		migrateForce       bool
		migrateRecursive   bool
		fixTitles          bool
		fixDates           bool
		fixTags            bool
		fixIDs             bool
		fixFilenames       bool
		preserveTimestamps bool
		indexSQLite        bool
		migrateAll         bool
		migrateGlobal      bool
		migrateWorkspace   string
		migrateBranch      string
		migrateType        string
		allWorkspaces      bool
		migrateVerbose     bool
		migrateShowReport  bool
		migrateNoBackup    bool
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
			// Initialize config and service
			config.InitConfig()
			svc, err := config.InitService()
			if err != nil {
				return err
			}
			defer svc.Close()

			if migrateAll {
				fixTitles = true
				fixDates = true
				fixTags = true
				fixIDs = true
				fixFilenames = true
				indexSQLite = true
			}

			if !fixTitles && !fixDates && !fixTags && !fixIDs && !fixFilenames && !indexSQLite {
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
				context, err := svc.GetWorkspaceContext("")
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

			report, err := migration.Migrate(basePath, options, os.Stdout)
			if err != nil {
				return fmt.Errorf("migration failed: %w", err)
			}

			if migrateShowReport {
				printMigrationReport(report, migrateDryRun)
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&migrateDryRun, "dry-run", false, "Show what would be changed without modifying")
	cmd.Flags().BoolVar(&migrateForce, "force", false, "Overwrite existing frontmatter")
	cmd.Flags().BoolVar(&migrateRecursive, "recursive", true, "Process directories recursively")
	cmd.Flags().BoolVar(&fixTitles, "fix-titles", false, "Extract titles from content if missing")
	cmd.Flags().BoolVar(&fixDates, "fix-dates", false, "Use file mtime if no date in frontmatter")
	cmd.Flags().BoolVar(&fixTags, "fix-tags", false, "Generate tags from path/repo/branch")
	cmd.Flags().BoolVar(&fixIDs, "fix-ids", false, "Generate missing IDs")
	cmd.Flags().BoolVar(&fixFilenames, "fix-filenames", false, "Standardize filenames to YYYYMMDD-title.md format")
	cmd.Flags().BoolVar(&preserveTimestamps, "preserve-timestamps", true, "Preserve original file modification times")
	cmd.Flags().BoolVar(&indexSQLite, "index-sqlite", false, "Create/update SQLite entries")
	cmd.Flags().BoolVar(&migrateAll, "all", false, "Apply all fixes")
	cmd.Flags().BoolVar(&migrateGlobal, globalStr, false, "Process global notes instead of current context")
	cmd.Flags().StringVar(&migrateWorkspace, "workspace", "", "Process specific workspace")
	cmd.Flags().StringVar(&migrateBranch, "branch", "", "Process specific branch")
	cmd.Flags().StringVar(&migrateType, "type", "", "Process only specific note type")
	cmd.Flags().BoolVar(&allWorkspaces, "all-workspaces", false, "Process all workspaces (requires confirmation)")
	cmd.Flags().BoolVar(&migrateVerbose, "verbose", false, "Show detailed output")
	cmd.Flags().BoolVar(&migrateShowReport, "report", true, "Show migration report")
	cmd.Flags().BoolVar(&migrateNoBackup, "no-backup", false, "Don't create backup files")

	// Add global flags
	config.AddGlobalFlags(cmd)

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