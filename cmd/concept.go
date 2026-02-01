package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/grovetools/nb/pkg/service"
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
	cmd.AddCommand(newConceptDirCmd(svc, workspaceOverride))
	cmd.AddCommand(newConceptPathCmd(svc, workspaceOverride))
	cmd.AddCommand(newConceptSearchCmd(svc, workspaceOverride))

	return cmd
}

func newConceptNewCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	var globalConcept bool
	var jsonOutput bool

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

			if jsonOutput {
				workspaceName := ""
				if ctx.NotebookContextWorkspace != nil {
					workspaceName = ctx.NotebookContextWorkspace.Name
				}
				if globalConcept {
					workspaceName = "global"
				}
				result := service.ConceptInfo{
					ID:        service.SanitizeFilename(title),
					Title:     title,
					Path:      note.Path,
					Workspace: workspaceName,
				}
				data, err := json.Marshal(result)
				if err != nil {
					return fmt.Errorf("marshal json: %w", err)
				}
				fmt.Println(string(data))
			} else {
				fmt.Printf("Created concept: %s\n", note.Path)
				fmt.Printf("  - concept-manifest.yml\n")
				fmt.Printf("  - overview.md\n")
			}
			return nil
		},
	}

	cmd.Flags().BoolVarP(&globalConcept, "global", "g", false, "Create in global workspace")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output result as JSON")
	return cmd
}

func newConceptListCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	var jsonOutput bool
	var allWorkspaces bool
	var ecosystem bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all concepts",
		Long: `List all concepts in the current workspace, ecosystem, or across all workspaces.

By default, lists concepts in the current workspace only.
Use --ecosystem to list concepts from all projects within the current ecosystem.
Use --all-workspaces to list concepts from all discovered workspaces globally.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var concepts []service.ConceptInfo
			var err error
			var showWorkspace bool

			ctx, ctxErr := (*svc).GetWorkspaceContext(*workspaceOverride)
			if ctxErr != nil {
				return fmt.Errorf("get workspace context: %w", ctxErr)
			}

			if allWorkspaces {
				concepts, err = (*svc).ListAllConcepts()
				showWorkspace = true
			} else if ecosystem {
				concepts, err = (*svc).ListEcosystemConcepts(ctx)
				showWorkspace = true
			} else {
				concepts, err = (*svc).ListConcepts(ctx)
				showWorkspace = false
			}
			if err != nil {
				return err
			}

			if jsonOutput {
				data, err := json.Marshal(concepts)
				if err != nil {
					return fmt.Errorf("marshal json: %w", err)
				}
				fmt.Println(string(data))
				return nil
			}

			if len(concepts) == 0 {
				fmt.Println("No concepts found.")
				return nil
			}

			fmt.Printf("Concepts (%d):\n", len(concepts))
			for _, concept := range concepts {
				if showWorkspace {
					fmt.Printf("  - %s (%s)\n", concept.ID, concept.Workspace)
				} else {
					fmt.Printf("  - %s\n", concept.ID)
				}
				if concept.Title != "" && concept.Title != concept.ID {
					fmt.Printf("    %s\n", concept.Title)
				}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output result as JSON")
	cmd.Flags().BoolVar(&ecosystem, "ecosystem", false, "List concepts from all projects within the current ecosystem")
	cmd.Flags().BoolVar(&allWorkspaces, "all-workspaces", false, "List concepts from all discovered workspaces globally")
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
	cmd.AddCommand(newConceptLinkSkillCmd(svc, workspaceOverride))

	return cmd
}

func newConceptLinkPlanCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	var jsonOutput bool

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

			if jsonOutput {
				result := map[string]interface{}{
					"success": true,
					"message": fmt.Sprintf("Linked plan '%s' to concept '%s'", planAlias, conceptID),
				}
				data, err := json.Marshal(result)
				if err != nil {
					return fmt.Errorf("marshal json: %w", err)
				}
				fmt.Println(string(data))
			} else {
				fmt.Printf("Linked plan '%s' to concept '%s'\n", planAlias, conceptID)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output result as JSON")
	return cmd
}

func newConceptLinkNoteCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	var jsonOutput bool

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

			if jsonOutput {
				result := map[string]interface{}{
					"success": true,
					"message": fmt.Sprintf("Linked note '%s' to concept '%s'", noteAlias, conceptID),
				}
				data, err := json.Marshal(result)
				if err != nil {
					return fmt.Errorf("marshal json: %w", err)
				}
				fmt.Println(string(data))
			} else {
				fmt.Printf("Linked note '%s' to concept '%s'\n", noteAlias, conceptID)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output result as JSON")
	return cmd
}

func newConceptLinkConceptCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	var jsonOutput bool

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

			if jsonOutput {
				result := map[string]interface{}{
					"success": true,
					"message": fmt.Sprintf("Linked concept '%s' to concept '%s'", targetID, sourceID),
				}
				data, err := json.Marshal(result)
				if err != nil {
					return fmt.Errorf("marshal json: %w", err)
				}
				fmt.Println(string(data))
			} else {
				fmt.Printf("Linked concept '%s' to concept '%s'\n", targetID, sourceID)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output result as JSON")
	return cmd
}

func newConceptLinkSkillCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "skill <concept-id> <skill-name>",
		Short: "Link a skill to a concept",
		Long:  `Add a skill reference to a concept's manifest. Use 'grove skills list' to see available skills.`,
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			conceptID := args[0]
			skillName := args[1]

			ctx, err := (*svc).GetWorkspaceContext(*workspaceOverride)
			if err != nil {
				return fmt.Errorf("get workspace context: %w", err)
			}

			if err := (*svc).LinkSkillToConcept(ctx, conceptID, skillName); err != nil {
				return err
			}

			if jsonOutput {
				result := map[string]interface{}{
					"success": true,
					"message": fmt.Sprintf("Linked skill '%s' to concept '%s'", skillName, conceptID),
				}
				data, err := json.Marshal(result)
				if err != nil {
					return fmt.Errorf("marshal json: %w", err)
				}
				fmt.Println(string(data))
			} else {
				fmt.Printf("Linked skill '%s' to concept '%s'\n", skillName, conceptID)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output result as JSON")
	return cmd
}

func newConceptDirCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dir",
		Short: "Get the concepts directory for the current workspace",
		Long:  `Returns the absolute path to the concepts directory in the notebook linked to the current workspace.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := (*svc).GetWorkspaceContext(*workspaceOverride)
			if err != nil {
				return fmt.Errorf("get workspace context: %w", err)
			}

			path, err := (*svc).GetConceptsDir(ctx)
			if err != nil {
				return err
			}

			fmt.Println(path)
			return nil
		},
	}

	return cmd
}

func newConceptPathCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "path <concept-id>",
		Short: "Get the path to a concept directory",
		Long:  `Returns the absolute path to a concept's directory.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			conceptID := args[0]

			ctx, err := (*svc).GetWorkspaceContext(*workspaceOverride)
			if err != nil {
				return fmt.Errorf("get workspace context: %w", err)
			}

			path, err := (*svc).GetConceptPath(ctx, conceptID)
			if err != nil {
				return err
			}

			if jsonOutput {
				result := map[string]string{"path": path}
				data, err := json.Marshal(result)
				if err != nil {
					return fmt.Errorf("marshal json: %w", err)
				}
				fmt.Println(string(data))
			} else {
				fmt.Println(path)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output result as JSON")
	return cmd
}

func newConceptSearchCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	var jsonOutput bool
	var filesOnly bool
	var ecosystem bool

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search concepts across all workspaces",
		Long:  `Search for a query string within concept files across the entire ecosystem.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := args[0]

			var results []service.ConceptSearchResult
			var err error

			if ecosystem {
				ctx, ctxErr := (*svc).GetWorkspaceContext(*workspaceOverride)
				if ctxErr != nil {
					return fmt.Errorf("get workspace context: %w", ctxErr)
				}
				results, err = (*svc).SearchEcosystemConcepts(ctx, query)
			} else {
				results, err = (*svc).SearchConcepts(query)
			}
			if err != nil {
				return err
			}

			if filesOnly {
				// Group files by concept ID
				type conceptFilesResult struct {
					Workspace string   `json:"workspace"`
					ConceptID string   `json:"concept_id"`
					Files     []string `json:"files"`
				}
				var grouped []conceptFilesResult
				conceptIndex := make(map[string]int)

				for _, result := range results {
					key := result.Workspace + "/" + result.ConceptID
					if idx, exists := conceptIndex[key]; exists {
						grouped[idx].Files = append(grouped[idx].Files, result.FilePath)
					} else {
						conceptIndex[key] = len(grouped)
						grouped = append(grouped, conceptFilesResult{
							Workspace: result.Workspace,
							ConceptID: result.ConceptID,
							Files:     []string{result.FilePath},
						})
					}
				}

				if jsonOutput {
					data, err := json.Marshal(grouped)
					if err != nil {
						return fmt.Errorf("marshal json: %w", err)
					}
					fmt.Println(string(data))
				} else {
					for _, g := range grouped {
						fmt.Printf("%s/%s:\n", g.Workspace, g.ConceptID)
						for _, fp := range g.Files {
							fmt.Printf("  %s\n", fp)
						}
					}
				}
				return nil
			}

			if jsonOutput {
				data, err := json.Marshal(results)
				if err != nil {
					return fmt.Errorf("marshal json: %w", err)
				}
				fmt.Println(string(data))
				return nil
			}

			if len(results) == 0 {
				fmt.Println("No matches found.")
				return nil
			}

			// Count total matches
			totalMatches := 0
			for _, r := range results {
				totalMatches += len(r.Matches)
			}

			fmt.Printf("Found %d match(es) in %d file(s):\n", totalMatches, len(results))
			for _, result := range results {
				fmt.Printf("\n  %s/%s: %s\n", result.Workspace, result.ConceptID, result.FilePath)
				for _, match := range result.Matches {
					fmt.Printf("    %d: %s\n", match.LineNumber, match.Text)
				}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&ecosystem, "ecosystem", false, "Search only within the current ecosystem")
	cmd.Flags().BoolVar(&filesOnly, "files-only", false, "Output only file paths, one per line")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output result as JSON")
	return cmd
}
