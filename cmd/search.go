package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	grovelogging "github.com/grovetools/core/logging"
	"github.com/grovetools/nb/pkg/models"
	"github.com/grovetools/nb/pkg/service"
)

var searchUlog = grovelogging.NewUnifiedLogger("grove-notebook.cmd.search")

func NewSearchCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	var (
		searchAll   bool
		searchType  string
		searchLimit int
	)

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search notes",
		Long: `Search for notes matching the query.

Examples:
  nb search "authentication"     # Search in current workspace
  nb search "todo" --all         # Search all workspaces
  nb search "api" -t llm         # Search only LLM notes`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s := *svc

			// Get workspace context
			ctx, err := s.GetWorkspaceContext(*workspaceOverride)
			if err != nil {
				return fmt.Errorf("get workspace context: %w", err)
			}

			query := strings.Join(args, " ")

			var opts []service.SearchOption
			if searchAll {
				opts = append(opts, service.AllWorkspaces())
			}
			if searchType != "" {
				opts = append(opts, service.OfType(models.NoteType(searchType)))
			}
			opts = append(opts, service.WithLimit(searchLimit))

			results, err := s.SearchNotes(ctx, query, opts...)
			if err != nil {
				return err
			}

			if len(results) == 0 {
				searchUlog.Info("No results found").
					Field("query", query).
					Pretty("No results found").
					PrettyOnly().
					Emit()
				return nil
			}

			searchUlog.Info("Search results").
				Field("query", query).
				Field("result_count", len(results)).
				Pretty(fmt.Sprintf("Found %d results:\n", len(results))).
				PrettyOnly().
				Emit()

			for i, note := range results {
				var prettyStr strings.Builder
				prettyStr.WriteString(fmt.Sprintf("%d. %s\n", i+1, note.Title))
				prettyStr.WriteString(fmt.Sprintf("   %s", note.Path))
				if note.Workspace != "" {
					prettyStr.WriteString(fmt.Sprintf("\n   Workspace: %s", note.Workspace))
					if note.Branch != "" {
						prettyStr.WriteString(fmt.Sprintf(" (branch: %s)", note.Branch))
					}
				}
				prettyStr.WriteString("\n")

				searchUlog.Info("Search result").
					Field("query", query).
					Field("result_index", i+1).
					Field("title", note.Title).
					Field("path", note.Path).
					Field("workspace", note.Workspace).
					Field("branch", note.Branch).
					Pretty(prettyStr.String()).
					PrettyOnly().
					Emit()
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&searchAll, "all", false, "Search all workspaces")
	cmd.Flags().StringVarP(&searchType, "type", "t", "", "Filter by note type")
	cmd.Flags().IntVar(&searchLimit, "limit", 50, "Maximum results")

	return cmd
}
