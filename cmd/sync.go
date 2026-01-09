package cmd

import (
	"context"
	"fmt"

	grovelogging "github.com/mattsolo1/grove-core/logging"
	"github.com/mattsolo1/grove-notebook/pkg/service"
	"github.com/mattsolo1/grove-notebook/pkg/sync"
	"github.com/mattsolo1/grove-notebook/pkg/sync/github"
	"github.com/spf13/cobra"
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

			return nil
		},
	}

	cmd.Flags().StringVar(&provider, "provider", "", "Sync only with a specific provider (e.g., github)")

	return cmd
}
