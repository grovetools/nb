package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/mattsolo1/grove-core/pkg/tmux"
	"github.com/mattsolo1/grove-notebook/pkg/service"
	"github.com/spf13/cobra"
)

// NewTmuxTuiCmd returns the command for opening nb tui in a tmux window.
func NewTmuxTuiCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	var windowName string

	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Open notebook TUI in a dedicated tmux window",
		Long: `Opens the nb TUI in a dedicated tmux window.
If the window already exists, it focuses it without disrupting the session.
If not in a tmux session, falls back to running the TUI directly.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := tmux.NewClient()
			if err != nil {
				// Not in a tmux session, run the TUI directly
				tuiCmd := NewTuiCmd(svc, workspaceOverride)
				return tuiCmd.RunE(cmd, args)
			}

			// Build the command to run in the tmux window
			nbBin, err := exec.LookPath("nb")
			if err != nil {
				nbBin = os.Args[0] // Fall back to current executable
			}

			command := fmt.Sprintf("%s tui", nbBin)

			// Use the tmux client to manage the window with error handling
			ctx := context.Background()
			if err := client.FocusOrRunTUIWithErrorHandling(ctx, command, windowName, -1); err != nil {
				return fmt.Errorf("failed to open in tmux window: %w", err)
			}

			// Close any popup that might have launched this command
			if err := client.ClosePopup(ctx); err != nil {
				// Ignore errors - we might not be in a popup
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&windowName, "window-name", "notebook", "Name of the tmux window")

	return cmd
}
