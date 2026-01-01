package cmd

import (
	"fmt"

	"github.com/mattsolo1/grove-notebook/pkg/service"
	"github.com/spf13/cobra"
)

func NewConceptCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "concept",
		Short: "Manage project concepts and architectural memory",
		Long:  `Create, list, and link project concepts to maintain durable architectural knowledge.`,
	}

	cmd.AddCommand(newConceptNewCmd(svc, workspaceOverride))
	cmd.AddCommand(newConceptListCmd(svc, workspaceOverride))
	cmd.AddCommand(newConceptLinkCmd(svc, workspaceOverride))

	return cmd
}

func newConceptNewCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	var globalConcept bool

	cmd := &cobra.Command{
		Use:   "new <title>",
		Short: "Create a new concept",
		Long:  `Create a new concept with manifest and overview files.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			title := args[0]
			ctx, err := (*svc).GetWorkspaceContext(*workspaceOverride)
			if err != nil {
				return fmt.Errorf("get workspace context: %w", err)
			}

			var opts []service.CreateOption
			if globalConcept {
				opts = append(opts, service.InGlobalWorkspace())
			}

			note, err := (*svc).CreateConcept(ctx, title, opts...)
			if err != nil {
				return err
			}

			fmt.Printf("Created concept: %s\n", note.Path)
			fmt.Printf("  - concept-manifest.yml\n")
			fmt.Printf("  - overview.md\n")
			return nil
		},
	}

	cmd.Flags().BoolVarP(&globalConcept, "global", "g", false, "Create in global workspace")
	return cmd
}

func newConceptListCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all concepts",
		Long:  `List all concepts in the current workspace.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := (*svc).GetWorkspaceContext(*workspaceOverride)
			if err != nil {
				return fmt.Errorf("get workspace context: %w", err)
			}

			concepts, err := (*svc).ListConcepts(ctx)
			if err != nil {
				return err
			}

			if len(concepts) == 0 {
				fmt.Println("No concepts found.")
				return nil
			}

			fmt.Printf("Concepts (%d):\n", len(concepts))
			for _, concept := range concepts {
				fmt.Printf("  - %s\n", concept.ID)
				if concept.Title != "" && concept.Title != concept.ID {
					fmt.Printf("    %s\n", concept.Title)
				}
			}
			return nil
		},
	}

	return cmd
}

func newConceptLinkCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "link",
		Short: "Link concepts, plans, and notes",
		Long:  `Link a concept to related concepts, plans, or notes.`,
	}

	cmd.AddCommand(newConceptLinkPlanCmd(svc, workspaceOverride))
	cmd.AddCommand(newConceptLinkNoteCmd(svc, workspaceOverride))
	cmd.AddCommand(newConceptLinkConceptCmd(svc, workspaceOverride))

	return cmd
}

func newConceptLinkPlanCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plan <concept-id> <plan-alias>",
		Short: "Link a plan to a concept",
		Long:  `Add a plan reference to a concept's manifest using an alias.`,
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			conceptID := args[0]
			planAlias := args[1]

			ctx, err := (*svc).GetWorkspaceContext(*workspaceOverride)
			if err != nil {
				return fmt.Errorf("get workspace context: %w", err)
			}

			if err := (*svc).LinkPlanToConcept(ctx, conceptID, planAlias); err != nil {
				return err
			}

			fmt.Printf("Linked plan '%s' to concept '%s'\n", planAlias, conceptID)
			return nil
		},
	}

	return cmd
}

func newConceptLinkNoteCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "note <concept-id> <note-alias>",
		Short: "Link a note to a concept",
		Long:  `Add a note reference to a concept's manifest using an alias.`,
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			conceptID := args[0]
			noteAlias := args[1]

			ctx, err := (*svc).GetWorkspaceContext(*workspaceOverride)
			if err != nil {
				return fmt.Errorf("get workspace context: %w", err)
			}

			if err := (*svc).LinkNoteToConcept(ctx, conceptID, noteAlias); err != nil {
				return err
			}

			fmt.Printf("Linked note '%s' to concept '%s'\n", noteAlias, conceptID)
			return nil
		},
	}

	return cmd
}

func newConceptLinkConceptCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "concept <source-concept-id> <target-concept-id>",
		Short: "Link a concept to another concept",
		Long:  `Add a concept-to-concept reference in the source concept's manifest.`,
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			sourceID := args[0]
			targetID := args[1]

			ctx, err := (*svc).GetWorkspaceContext(*workspaceOverride)
			if err != nil {
				return fmt.Errorf("get workspace context: %w", err)
			}

			if err := (*svc).LinkConceptToConcept(ctx, sourceID, targetID); err != nil {
				return err
			}

			fmt.Printf("Linked concept '%s' to concept '%s'\n", targetID, sourceID)
			return nil
		},
	}

	return cmd
}
