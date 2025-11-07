package cmd

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"
	"github.com/mattsolo1/grove-notebook/cmd/config"
	"github.com/mattsolo1/grove-notebook/internal/tui/browser"
	"github.com/spf13/cobra"
)

// NewTuiCmd creates the `nb tui` command.
func NewTuiCmd() *cobra.Command {
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

			// Initialize config and service
			config.InitConfig()
			svc, err := config.InitService()
			if err != nil {
				return err
			}
			defer svc.Close()

			// Create and run TUI
			model := browser.New(svc)
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
