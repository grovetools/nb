package cmd

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-notebook/internal/tui/browser"
	"github.com/mattsolo1/grove-notebook/pkg/service"
	"github.com/spf13/cobra"
)

// NewTuiCmd creates the `nb tui` command.
func NewTuiCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Launch an interactive TUI for browsing notes across workspaces",
		Long: `Launch an interactive Terminal User Interface for browsing and managing notes.
This view provides a workspace-centric way to explore your entire notebook.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Check for TTY
			if !isatty.IsTerminal(os.Stdout.Fd()) && !isatty.IsCygwinTerminal(os.Stdout.Fd()) {
				return fmt.Errorf("TUI mode requires an interactive terminal")
			}

			s := *svc

			// Get current workspace context to determine initial focus
			ctx, err := s.GetWorkspaceContext(*workspaceOverride)
			if err != nil {
				return fmt.Errorf("failed to get workspace context: %w", err)
			}

			var initialFocus *workspace.WorkspaceNode
			if ctx.NotebookContextWorkspace.Name != "global" {
				initialFocus = ctx.NotebookContextWorkspace
			}

			// Create and run TUI with initial focus
			model := browser.New(s, initialFocus)
			p := tea.NewProgram(model, tea.WithAltScreen())

			if _, err := p.Run(); err != nil {
				return fmt.Errorf("error running TUI: %w", err)
			}

			return nil
		},
	}
	return cmd
}
