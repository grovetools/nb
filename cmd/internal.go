package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// NewInternalCmd creates the root 'internal' command, which is hidden from the user.
func NewInternalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "internal",
		Short:  "Internal commands for automation (not for direct use)",
		Hidden: true,
	}

	cmd.AddCommand(newUpdateNoteCmd())

	return cmd
}

// newUpdateNoteCmd creates the 'internal update-note' command.
func newUpdateNoteCmd() *cobra.Command {
	var (
		notePath      string
		appendContent string
	)

	cmd := &cobra.Command{
		Use:   "update-note",
		Short: "Appends content to a specific note file",
		RunE: func(cmd *cobra.Command, args []string) error {
			if notePath == "" {
				return fmt.Errorf("--path is required")
			}
			if appendContent == "" {
				return fmt.Errorf("--append-content is required")
			}

			// Open the file in append mode
			f, err := os.OpenFile(notePath, os.O_APPEND|os.O_WRONLY, 0644)
			if err != nil {
				return fmt.Errorf("failed to open note file for appending: %w", err)
			}
			defer f.Close()

			// Write the content
			if _, err := f.WriteString(appendContent); err != nil {
				return fmt.Errorf("failed to append content to note: %w", err)
			}

			fmt.Printf("✓ Content appended to %s\n", notePath)
			return nil
		},
	}

	cmd.Flags().StringVar(&notePath, "path", "", "The absolute path to the note file to update")
	cmd.Flags().StringVar(&appendContent, "append-content", "", "The content to append to the note")
	_ = cmd.MarkFlagRequired("path")
	_ = cmd.MarkFlagRequired("append-content")

	return cmd
}
