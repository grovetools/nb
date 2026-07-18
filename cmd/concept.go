package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/spf13/cobra"

	"github.com/grovetools/nb/pkg/service"
)

func NewConceptCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "concept",
		Short: "Manage project concepts and architectural memory",
		Long:  `Create, list, and link project concepts to maintain durable architectural knowledge.`,
		Example: `  # Find concepts related to authentication
  nb concept search "auth" --ecosystem --files-only --json

  # List all concepts in the ecosystem
  nb concept list --ecosystem --json

  # Get the path to a concept and read it
  nb concept path authentication
  cat $(nb concept path authentication)/overview.md

  # Create a new concept and link it
  nb concept new "Rate Limiter" --json
  nb concept link plan rate-limiter myproject:plans/implement-rate-limit
  nb concept link concept rate-limiter core:workspace-model`,
	}

	cmd.AddCommand(newConceptNewCmd(svc, workspaceOverride))
	cmd.AddCommand(newConceptListCmd(svc, workspaceOverride))
	cmd.AddCommand(newConceptLinkCmd(svc, workspaceOverride))
	cmd.AddCommand(newConceptDirCmd(svc, workspaceOverride))
	cmd.AddCommand(newConceptPathCmd(svc, workspaceOverride))
	cmd.AddCommand(newConceptSearchCmd(svc, workspaceOverride))
	cmd.AddCommand(newConceptGapCmd(svc, workspaceOverride))

	return cmd
}

func newConceptNewCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	var globalConcept bool
	var jsonOutput bool
	var conceptID string

	cmd := &cobra.Command{
		Use:   "new <title>",
		Short: "Create a new concept",
		Long:  `Create a new concept with manifest and overview files.`,
		Example: `  nb concept new "Rate Limiter"
  nb concept new "Authentication System" --json
  nb concept new "Rate Limiter: Token Buckets" --id rate-limiter
  nb concept new "Shared Utils" --global`,
		Args: cobra.ExactArgs(1),
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
			if conceptID != "" {
				opts = append(opts, service.WithConceptID(conceptID))
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
					workspaceName = globalStr
				}
				resultID := conceptID
				if resultID == "" {
					resultID = service.SanitizeFilename(title)
				}
				result := service.ConceptInfo{
					ID:        resultID,
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
	cmd.Flags().StringVar(&conceptID, "id", "", "Explicit concept id (directory name); defaults to a slug of the title")
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
		Example: `  nb concept list
  nb concept list --ecosystem --json
  nb concept list --all-workspaces --json`,
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
		Example: `  nb concept link plan auth myproject:plans/jwt-refactor
  nb concept link note auth myproject:inbox/auth-history.md
  nb concept link concept auth grovetools:session-tracking
  nb concept link skill auth concept-maintainer`,
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
		Example: `  nb concept link plan rate-limiter myproject:plans/implement-rate-limit
  nb concept link plan authentication flow:plans/jwt-refactor --json`,
		Args: cobra.ExactArgs(2),
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
		Example: `  nb concept link note authentication myproject:inbox/auth-history.md
  nb concept link note rate-limiter myproject:inbox/rfc.md --json`,
		Args: cobra.ExactArgs(2),
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
		Long: `Add a concept-to-concept reference in the source concept's manifest.
Use workspace:concept-id format to link concepts across workspaces.`,
		Example: `  nb concept link concept auth grovetools:session-tracking
  nb concept link concept rate-limiter core:workspace-model --json`,
		Args: cobra.ExactArgs(2),
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
		Example: `  grove skills list                              # discover available skills
  nb concept link skill auth concept-maintainer
  nb concept link skill flow-execution flow-qb --json`,
		Args: cobra.ExactArgs(2),
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
		Example: `  nb concept dir
  ls $(nb concept dir)`,
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
		Example: `  nb concept path authentication
  cat $(nb concept path authentication)/overview.md
  nb concept path auth --json | jq -r .path`,
		Args: cobra.ExactArgs(1),
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
	var limit int
	var searchIn string

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search concepts across all workspaces",
		Long: `Search for a query within concept files and rank the matching concepts.

Multi-word queries are tokenized on whitespace: concepts matching ANY token are
returned, ranked by token coverage and where the hits land (manifest title
beats description beats overview.md beats other files). Tokens are matched as
literal case-insensitive substrings, never as regexes.`,
		Example: `  nb concept search "auth"
  nb concept search "model resolution" --ecosystem --json
  nb concept search "session" --in role --limit 5
  nb concept search "workspace" --ecosystem --files-only --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := args[0]
			opts := service.ConceptSearchOptions{In: searchIn, Limit: limit}

			var results []service.ConceptSearchResult
			var err error

			if ecosystem {
				ctx, ctxErr := (*svc).GetWorkspaceContext(*workspaceOverride)
				if ctxErr != nil {
					return fmt.Errorf("get workspace context: %w", ctxErr)
				}
				results, err = (*svc).SearchEcosystemConcepts(ctx, query, opts)
			} else {
				results, err = (*svc).SearchConcepts(query, opts)
			}
			if err != nil {
				return err
			}

			if filesOnly {
				for i := range results {
					for j := range results[i].Files {
						results[i].Files[j].Matches = nil
					}
				}
			}

			if jsonOutput {
				if results == nil {
					results = []service.ConceptSearchResult{}
				}
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

			fmt.Printf("Found %d concept(s):\n", len(results))
			for _, result := range results {
				title := result.Title
				if title == "" {
					title = result.ConceptID
				}
				fmt.Printf("\n%s/%s — %s (score %.2f)\n", result.Workspace, result.ConceptID, title, result.Score)
				if result.Description != "" {
					fmt.Printf("  %s\n", result.Description)
				}
				for _, file := range result.Files {
					if filesOnly {
						fmt.Printf("  %s\n", file.FilePath)
						continue
					}
					for _, match := range file.Matches {
						fmt.Printf("  %s:%d: %s\n", file.FilePath, match.LineNumber, match.Text)
					}
				}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&ecosystem, "ecosystem", false, "Search only within the current ecosystem")
	cmd.Flags().BoolVar(&filesOnly, "files-only", false, "Omit line matches; list matched files only")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output result as JSON")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum number of concepts to return (0 = unlimited)")
	cmd.Flags().StringVar(&searchIn, "in", "all", "Search scope: role (manifest title/description + overview.md), overview (overview.md only), all (every concept file)")
	return cmd
}

func newConceptGapCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gap",
		Short: "Record and triage concept context gaps",
		Long: `Track files flagged as essential-but-missing from a concept's derived context.

Gaps are appended to a .gaps.jsonl store in the workspace's concepts directory,
then triaged (e.g. by grove-concept-maintainer) into preset additions or new
concepts.`,
		Example: `  nb concept gap record --file pkg/context/resolve.go --concept concept-knowledge-base --reason "resolver is load-bearing" --source flow-context-tuner
  nb concept gap record --stdin < gaps.json
  nb concept gap list --json
  nb concept gap resolve 1a2b3c4d5e6f --as added-to-preset`,
	}

	cmd.AddCommand(newConceptGapRecordCmd(svc, workspaceOverride))
	cmd.AddCommand(newConceptGapListCmd(svc, workspaceOverride))
	cmd.AddCommand(newConceptGapResolveCmd(svc, workspaceOverride))

	return cmd
}

func newConceptGapRecordCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	var file, conceptID, reason, source string
	var fromStdin, jsonOutput bool

	cmd := &cobra.Command{
		Use:   "record",
		Short: "Record a concept context gap",
		Long: `Record a file as essential-but-missing from a concept's derived context.

The gap kind is derived automatically: "preset" when --concept names the owning
concept (its preset is missing the file), "concept" when no concept covers the
file. With --stdin, a JSON array of gaps is read from standard input so raters
can batch: [{"file":"...","concept_id":"...","reason":"..."}].`,
		Example: `  nb concept gap record --file pkg/context/resolve.go --concept concept-knowledge-base --reason "resolver is load-bearing"
  nb concept gap record --file pkg/orphan/thing.go --reason "nothing covers this" --json
  echo '[{"file":"a.go","concept_id":"auth","reason":"core"}]' | nb concept gap record --stdin`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := (*svc).GetWorkspaceContext(*workspaceOverride)
			if err != nil {
				return fmt.Errorf("get workspace context: %w", err)
			}

			var pending []service.ConceptGap
			if fromStdin {
				data, err := io.ReadAll(cmd.InOrStdin())
				if err != nil {
					return fmt.Errorf("read stdin: %w", err)
				}
				if err := json.Unmarshal(data, &pending); err != nil {
					return fmt.Errorf("parse stdin gap batch: %w", err)
				}
				for i := range pending {
					if pending[i].Source == "" {
						pending[i].Source = source
					}
				}
			} else {
				if file == "" {
					return fmt.Errorf("either --file or --stdin is required")
				}
				pending = []service.ConceptGap{{
					File:      file,
					ConceptID: conceptID,
					Reason:    reason,
					Source:    source,
				}}
			}

			recorded := make([]service.ConceptGap, 0, len(pending))
			for _, gap := range pending {
				rec, err := (*svc).RecordConceptGap(ctx, gap)
				if err != nil {
					return err
				}
				recorded = append(recorded, rec)
			}

			if jsonOutput {
				var payload interface{} = recorded
				if !fromStdin {
					payload = recorded[0]
				}
				data, err := json.Marshal(payload)
				if err != nil {
					return fmt.Errorf("marshal json: %w", err)
				}
				fmt.Println(string(data))
				return nil
			}

			for _, rec := range recorded {
				target := rec.ConceptID
				if target == "" {
					target = "(unowned)"
				}
				fmt.Printf("Recorded gap %s (%s) for %s: %s\n", rec.ID, rec.Kind, target, rec.File)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&file, "file", "", "The essential-but-missing file (abs or workspace-rel)")
	cmd.Flags().StringVar(&conceptID, "concept", "", "Owning concept id, if known (omit when nothing covers the file)")
	cmd.Flags().StringVar(&reason, "reason", "", "Why the file is essential")
	cmd.Flags().StringVar(&source, "source", "manual", "Who flagged the gap (e.g. flow-context-tuner, a job id)")
	cmd.Flags().BoolVar(&fromStdin, "stdin", false, "Read a JSON array of gaps from stdin")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output result as JSON")
	return cmd
}

func newConceptGapListCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	var all, resolved, jsonOutput bool
	var conceptID string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List recorded concept gaps",
		Long:  `List concept gaps from the workspace's gap store. By default only unresolved gaps are shown.`,
		Example: `  nb concept gap list
  nb concept gap list --concept concept-knowledge-base --json
  nb concept gap list --all
  nb concept gap list --resolved`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := (*svc).GetWorkspaceContext(*workspaceOverride)
			if err != nil {
				return fmt.Errorf("get workspace context: %w", err)
			}

			gaps, err := (*svc).ListConceptGaps(ctx, service.ConceptGapListFilter{
				IncludeResolved: all,
				OnlyResolved:    resolved,
				ConceptID:       conceptID,
			})
			if err != nil {
				return err
			}

			if jsonOutput {
				if gaps == nil {
					gaps = []service.ConceptGap{}
				}
				data, err := json.Marshal(gaps)
				if err != nil {
					return fmt.Errorf("marshal json: %w", err)
				}
				fmt.Println(string(data))
				return nil
			}

			if len(gaps) == 0 {
				fmt.Println("No gaps found.")
				return nil
			}

			// Group by concept, "(unowned)" last.
			grouped := make(map[string][]service.ConceptGap)
			var order []string
			for _, gap := range gaps {
				key := gap.ConceptID
				if _, seen := grouped[key]; !seen && key != "" {
					order = append(order, key)
				}
				grouped[key] = append(grouped[key], gap)
			}
			sort.Strings(order)
			if _, hasUnowned := grouped[""]; hasUnowned {
				order = append(order, "")
			}

			fmt.Printf("Gaps (%d):\n", len(gaps))
			for _, key := range order {
				label := key
				if label == "" {
					label = "(unowned)"
				}
				fmt.Printf("\n%s:\n", label)
				for _, gap := range grouped[key] {
					status := ""
					if gap.Resolved {
						status = fmt.Sprintf(" [resolved: %s]", gap.ResolvedBy)
					}
					fmt.Printf("  %s  %s%s\n", gap.ID, gap.File, status)
					if gap.Reason != "" {
						fmt.Printf("      %s\n", gap.Reason)
					}
					fmt.Printf("      %s · %s\n", gap.Source, gap.CreatedAt.Format("2006-01-02"))
				}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "Include resolved gaps")
	cmd.Flags().BoolVar(&resolved, "resolved", false, "Show resolved gaps only")
	cmd.Flags().StringVar(&conceptID, "concept", "", "Restrict to one concept")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output result as JSON")
	return cmd
}

func newConceptGapResolveCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	var resolvedBy string
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "resolve <id>",
		Short: "Mark a concept gap as resolved",
		Long:  `Mark a gap as resolved by its id (a unique id prefix is accepted).`,
		Example: `  nb concept gap resolve 1a2b3c4d5e6f --as added-to-preset
  nb concept gap resolve 1a2b --as new-concept:context-resolution
  nb concept gap resolve 1a2b --as rejected --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := (*svc).GetWorkspaceContext(*workspaceOverride)
			if err != nil {
				return fmt.Errorf("get workspace context: %w", err)
			}

			gap, err := (*svc).ResolveConceptGap(ctx, args[0], resolvedBy)
			if err != nil {
				return err
			}

			if jsonOutput {
				data, err := json.Marshal(gap)
				if err != nil {
					return fmt.Errorf("marshal json: %w", err)
				}
				fmt.Println(string(data))
				return nil
			}

			fmt.Printf("Resolved gap %s: %s", gap.ID, gap.File)
			if gap.ResolvedBy != "" {
				fmt.Printf(" (%s)", gap.ResolvedBy)
			}
			fmt.Println()
			return nil
		},
	}

	cmd.Flags().StringVar(&resolvedBy, "as", "", "How the gap was resolved: added-to-preset, new-concept:<id>, or rejected")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output result as JSON")
	return cmd
}
