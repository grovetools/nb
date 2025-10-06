package cmd

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mattsolo1/grove-notebook/cmd/config"
	"github.com/mattsolo1/grove-notebook/pkg/workspace"
	"github.com/spf13/cobra"
	_ "github.com/mattn/go-sqlite3"
)

// OldWorkspace represents the structure in the legacy workspaces.db
type OldWorkspace struct {
	Name        string
	Path        string
	Type        string
	NotebookDir string
	Settings    string
	CreatedAt   time.Time
	LastUsed    time.Time
}

func newWorkspaceMigrateDbCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate-db",
		Short: "Migrate legacy nb workspaces to the Grove workspace system",
		Long: `This command reads workspace data from the old nb-specific SQLite database
(~/.local/share/nb/workspaces.db) and migrates it to the new centralized
Grove workspace management system.

This should be run once after updating to a version of nb that uses Grove workspaces.
The old database will be backed up after a successful migration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Migrating legacy nb workspaces to Grove workspaces...")

			// Initialize a new service to get access to the new registry
			config.InitConfig()
			svc, err := config.InitService()
			if err != nil {
				return fmt.Errorf("failed to initialize service: %w", err)
			}
			defer svc.Close()

			// Path to the old database
			home, _ := os.UserHomeDir()
			oldDbPath := filepath.Join(home, ".local", "share", "nb", "workspaces.db")
			backupDbPath := oldDbPath + ".bak"

			if _, err := os.Stat(oldDbPath); os.IsNotExist(err) {
				fmt.Println("No legacy database found. Nothing to migrate.")
				return nil
			}

			// Connect to the old database
			oldDb, err := sql.Open("sqlite3", oldDbPath)
			if err != nil {
				return fmt.Errorf("failed to open old database: %w", err)
			}
			defer oldDb.Close()

			rows, err := oldDb.Query("SELECT name, path, type, notebook_dir, settings, created_at, last_used FROM workspaces")
			if err != nil {
				return fmt.Errorf("failed to query old workspaces: %w", err)
			}
			defer rows.Close()

			var oldWorkspaces []*OldWorkspace
			for rows.Next() {
				var ow OldWorkspace
				if err := rows.Scan(&ow.Name, &ow.Path, &ow.Type, &ow.NotebookDir, &ow.Settings, &ow.CreatedAt, &ow.LastUsed); err != nil {
					return fmt.Errorf("failed to scan old workspace row: %w", err)
				}
				oldWorkspaces = append(oldWorkspaces, &ow)
			}

			if len(oldWorkspaces) == 0 {
				fmt.Println("No workspaces found in the legacy database.")
				// Still rename the file to prevent re-running
				return os.Rename(oldDbPath, backupDbPath)
			}

			fmt.Printf("Found %d legacy workspaces to migrate.\n", len(oldWorkspaces))

			migratedCount := 0
			for _, ow := range oldWorkspaces {
				fmt.Printf("Migrating '%s'...", ow.Name)

				var settings map[string]interface{}
				if ow.Settings != "" {
					if err := json.Unmarshal([]byte(ow.Settings), &settings); err != nil {
						fmt.Printf(" failed to parse settings: %v. Skipping.\n", err)
						continue
					}
				}

				newWs := &workspace.Workspace{
					Name:        ow.Name,
					Path:        ow.Path,
					Type:        workspace.Type(ow.Type),
					NotebookDir: ow.NotebookDir,
					Settings:    settings,
					CreatedAt:   ow.CreatedAt,
					LastUsed:    ow.LastUsed,
				}

				if err := svc.Registry.Add(newWs); err != nil {
					fmt.Printf(" failed to add to new registry: %v. Skipping.\n", err)
					continue
				}
				migratedCount++
				fmt.Println(" done.")
			}

			fmt.Printf("Successfully migrated %d out of %d workspaces.\n", migratedCount, len(oldWorkspaces))

			// Backup the old database
			fmt.Printf("Backing up old database to %s\n", backupDbPath)
			if err := os.Rename(oldDbPath, backupDbPath); err != nil {
				return fmt.Errorf("failed to back up old database: %w", err)
			}

			fmt.Println("Migration complete.")
			return nil
		},
	}

	return cmd
}
