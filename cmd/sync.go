package cmd

import (
	"fmt"

	"github.com/mattsolo1/grove-notebook/pkg/service"
	"github.com/mattsolo1/grove-notebook/pkg/sync"
	"github.com/mattsolo1/grove-notebook/pkg/sync/github"
	"github.com/spf13/cobra"
)

// NewSyncCmd creates the `sync` subcommand.
func NewSyncCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	var provider string

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync notes with remote services",
		Long:  `Syncs notes with configured remote services like GitHub issues and pull requests.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			s := *svc
			ctx, err := s.GetWorkspaceContext(*workspaceOverride)
			if err != nil {
				return fmt.Errorf("get workspace context: %w", err)
			}

			// Create syncer and register providers
			syncer := sync.NewSyncer(s)
			syncer.RegisterProvider("github", func() sync.Provider {
				return github.NewProvider()
			})

			// Run sync
			reports, err := syncer.SyncWorkspace(ctx)
			if err != nil {
				return err
			}

			// Display results
			for _, report := range reports {
				fmt.Printf("Synced with %s: %d created, %d updated, %d unchanged, %d failed.\n",
					report.Provider, report.Created, report.Updated, report.Unchanged, report.Failed)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&provider, "provider", "", "Sync only with a specific provider (e.g., github)")

	return cmd
}
