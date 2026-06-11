package cmd

import (
	"context"
	"fmt"

	grovelogging "github.com/grovetools/core/logging"
	"github.com/spf13/cobra"

	"github.com/grovetools/nb/pkg/service"
	"github.com/grovetools/nb/pkg/sync"
	"github.com/grovetools/nb/pkg/sync/github"
)

var syncUlog = grovelogging.NewUnifiedLogger("grove-notebook.cmd.sync")

// NewSyncCmd creates the `sync` subcommand.
func NewSyncCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	var provider string

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync notes with remote services",
		Long:  `Syncs notes with configured remote services like GitHub issues and pull requests.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			s := *svc
			wsCtx, err := s.GetWorkspaceContext(*workspaceOverride)
			if err != nil {
				return fmt.Errorf("get workspace context: %w", err)
			}

			// Create syncer and register providers
			syncer := sync.NewSyncer(s)
			syncer.RegisterProvider("github", func() sync.Provider {
				return github.NewProvider()
			})

			// Run sync
			reports, err := syncer.SyncWorkspace(wsCtx)
			if err != nil {
				return err
			}

			// Display results
			for _, report := range reports {
				syncUlog.Success("Sync complete").
					Field("provider", report.Provider).
					Field("created", report.Created).
					Field("updated", report.Updated).
					Field("unchanged", report.Unchanged).
					Field("failed", report.Failed).
					Pretty(fmt.Sprintf("Synced with %s: %d created, %d updated, %d unchanged, %d failed.",
						report.Provider, report.Created, report.Updated, report.Unchanged, report.Failed)).
					PrettyOnly().
					Log(ctx)
				// Show error details if there were any failures
				if len(report.Errors) > 0 {
					syncUlog.Error("Sync errors encountered").
						Field("provider", report.Provider).
						Field("error_count", len(report.Errors)).
						Pretty("Errors:").
						PrettyOnly().
						Log(ctx)
					for _, errMsg := range report.Errors {
						syncUlog.Error("Sync error").
							Field("provider", report.Provider).
							Field("error", errMsg).
							Pretty(fmt.Sprintf("  - %s", errMsg)).
							PrettyOnly().
							Log(ctx)
					}
				}
			}

			notifyDaemonRefreshCmd()

			return nil
		},
	}

	cmd.Flags().StringVar(&provider, "provider", "", "Sync only with a specific provider (e.g., github)")

	// Add subcommands for Notebook Sync Phase 2 (daemon-coordinated)
	cmd.AddCommand(NewSyncHistoryCmd(svc, workspaceOverride))
	cmd.AddCommand(NewSyncRestoreCmd(svc, workspaceOverride))
	cmd.AddCommand(NewSyncConflictsCmd(svc, workspaceOverride))

	return cmd
}

// NewSyncHistoryCmd creates the `sync history` subcommand.
// Displays the version history for a document from the sync server.
func NewSyncHistoryCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	return &cobra.Command{
		Use:   "history <file>",
		Short: "Show version history for a synced document",
		Long:  `Displays the version history of a document stored on the sync server, allowing you to see who changed it and when.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: Call daemon's /api/sync/history endpoint via Unix socket
			// For now, just show a placeholder
			fmt.Printf("Version history for: %s\n", args[0])
			fmt.Println("(History integration with daemon pending)")
			return nil
		},
	}
}

// NewSyncRestoreCmd creates the `sync restore` subcommand.
// Restores a document from a specific version.
func NewSyncRestoreCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	var version string
	cmd := &cobra.Command{
		Use:   "restore <file>",
		Short: "Restore a document to a specific version",
		Long:  `Restores a document from the sync server history, allowing you to undo unwanted changes.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: Call daemon's /api/sync/restore endpoint via Unix socket
			// For now, just show a placeholder
			fmt.Printf("Restore %s to version: %s\n", args[0], version)
			fmt.Println("(Restore integration with daemon pending)")
			return nil
		},
	}
	cmd.Flags().StringVar(&version, "version", "", "Version ID to restore to (required)")
	return cmd
}

// NewSyncConflictsCmd creates the `sync conflicts` subcommand.
// Lists, views, and resolves merge conflicts from sync.
func NewSyncConflictsCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	return &cobra.Command{
		Use:   "conflicts",
		Short: "Manage sync conflicts",
		Long:  `List, view, and resolve conflicts from document synchronization.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: List conflicts from ~/.local/state/grove/sync/conflicts/
			// For now, just show a placeholder
			fmt.Println("Sync conflicts:")
			fmt.Println("(Conflicts integration with daemon pending)")
			return nil
		},
	}
}
