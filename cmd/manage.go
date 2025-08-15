package cmd

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattsolo1/grove-notebook/cmd/config"
	"github.com/mattsolo1/grove-notebook/internal/tui/manager"
	"github.com/spf13/cobra"
)

func NewManageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "manage",
		Short: "Interactively manage and archive notes in the current workspace",
		Long: `Provides an interactive TUI to view and manage notes in the current workspace context.
You can select multiple notes for archiving or other bulk operations.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			config.InitConfig()
			svc, err := config.InitService()
			if err != nil {
				return fmt.Errorf("failed to initialize service: %w", err)
			}
			defer svc.Close()

			// Get the current workspace context
			ctx, err := svc.GetWorkspaceContext()
			if err != nil {
				return fmt.Errorf("failed to get workspace context: %w", err)
			}

			// Get notes from the current workspace context
			notes, err := svc.ListAllNotes(ctx)
			if err != nil {
				return fmt.Errorf("failed to list notes: %w", err)
			}

			// Create and run the TUI
			model := manager.New(notes, svc, ctx)
			p := tea.NewProgram(model, tea.WithAltScreen())

			if _, err := p.Run(); err != nil {
				return fmt.Errorf("error running TUI: %w", err)
			}

			return nil
		},
	}

	config.AddGlobalFlags(cmd)
	return cmd
}