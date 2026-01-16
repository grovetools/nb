package cmd

import (
	"fmt"
	"os"

	"github.com/grovetools/nb/pkg/frontmatter"
	"github.com/grovetools/nb/pkg/service"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	grovelogging "github.com/grovetools/core/logging"
)

var internalUlog = grovelogging.NewUnifiedLogger("grove-notebook.cmd.internal")

// NewInternalCmd creates the root 'internal' command, which is hidden from the user.
func NewInternalCmd(svc **service.Service) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "internal",
		Short:  "Internal commands for automation (not for direct use)",
		Hidden: true,
	}

	cmd.AddCommand(newUpdateNoteCmd(svc))
	cmd.AddCommand(newUpdateFrontmatterCmd(svc))

	return cmd
}

// newUpdateNoteCmd creates the 'internal update-note' command.
func newUpdateNoteCmd(svc **service.Service) *cobra.Command {
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

			(*svc).Logger.WithField("path", notePath).Info("Appended content to note")
			internalUlog.Success("Content appended").
				Field("path", notePath).
				Field("content_length", len(appendContent)).
				Pretty(fmt.Sprintf("* Content appended to %s", notePath)).
				PrettyOnly().
				Emit()
			return nil
		},
	}

	cmd.Flags().StringVar(&notePath, "path", "", "The absolute path to the note file to update")
	cmd.Flags().StringVar(&appendContent, "append-content", "", "The content to append to the note")
	_ = cmd.MarkFlagRequired("path")
	_ = cmd.MarkFlagRequired("append-content")

	return cmd
}

// newUpdateFrontmatterCmd creates the 'internal update-frontmatter' command.
func newUpdateFrontmatterCmd(svc **service.Service) *cobra.Command {
	var (
		notePath  string
		fieldName string
		fieldValue string
	)

	cmd := &cobra.Command{
		Use:   "update-frontmatter",
		Short: "Updates a frontmatter field in a specific note file",
		RunE: func(cmd *cobra.Command, args []string) error {
	if notePath == "" {
				return fmt.Errorf("--path is required")
			}
			if fieldName == "" {
				return fmt.Errorf("--field is required")
			}
			if fieldValue == "" {
				return fmt.Errorf("--value is required")
			}

			// Read the note file
			content, err := os.ReadFile(notePath)
			if err != nil {
				return fmt.Errorf("failed to read note file: %w", err)
			}

			// Parse frontmatter
			fm, body, err := frontmatter.Parse(string(content))
			if err != nil {
				return fmt.Errorf("failed to parse frontmatter: %w", err)
			}
			if fm == nil {
				return fmt.Errorf("note has no frontmatter")
			}

			// Update the specified field
			switch fieldName {
			case "plan_ref":
				fm.PlanRef = fieldValue
			case "title":
				fm.Title = fieldValue
			case "repository":
				fm.Repository = fieldValue
			case "branch":
				fm.Branch = fieldValue
			case "worktree":
				fm.Worktree = fieldValue
			default:
				return fmt.Errorf("unsupported field: %s", fieldName)
			}

			// Rebuild content with updated frontmatter
			newContent := frontmatter.BuildContent(fm, body)

			// Write back to file
			if err := os.WriteFile(notePath, []byte(newContent), 0644); err != nil {
				return fmt.Errorf("failed to write note file: %w", err)
			}

			(*svc).Logger.WithFields(logrus.Fields{
				"path":  notePath,
				"field": fieldName,
				"value": fieldValue,
			}).Info("Updated note frontmatter")
			internalUlog.Success("Frontmatter updated").
				Field("path", notePath).
				Field("field", fieldName).
				Field("value", fieldValue).
				Pretty(fmt.Sprintf("* Updated %s in %s", fieldName, notePath)).
				PrettyOnly().
				Emit()
			return nil
		},
	}

	cmd.Flags().StringVar(&notePath, "path", "", "The absolute path to the note file to update")
	cmd.Flags().StringVar(&fieldName, "field", "", "The frontmatter field to update (e.g., plan_ref)")
	cmd.Flags().StringVar(&fieldValue, "value", "", "The value to set for the field")
	_ = cmd.MarkFlagRequired("path")
	_ = cmd.MarkFlagRequired("field")
	_ = cmd.MarkFlagRequired("value")

	return cmd
}
