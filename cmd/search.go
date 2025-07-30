package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mattsolo1/grove-notebook/cmd/config"
	"github.com/mattsolo1/grove-notebook/pkg/models"
	"github.com/mattsolo1/grove-notebook/pkg/service"
)

func NewSearchCmd() *cobra.Command {
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
			// Initialize config and service
			config.InitConfig()
			svc, err := config.InitService()
			if err != nil {
				return err
			}
			defer svc.Close()

			// Get workspace context
			ctx, err := svc.GetWorkspaceContext()
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

			results, err := svc.SearchNotes(ctx, query, opts...)
			if err != nil {
				return err
			}

			if len(results) == 0 {
				fmt.Println("No results found")
				return nil
			}

			fmt.Printf("Found %d results:\n\n", len(results))

			for i, note := range results {
				fmt.Printf("%d. %s\n", i+1, note.Title)
				fmt.Printf("   %s\n", note.Path)
				if note.Workspace != "" {
					fmt.Printf("   Workspace: %s", note.Workspace)
					if note.Branch != "" {
						fmt.Printf(" (branch: %s)", note.Branch)
					}
					fmt.Println()
				}
				fmt.Println()
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&searchAll, "all", false, "Search all workspaces")
	cmd.Flags().StringVarP(&searchType, "type", "t", "", "Filter by note type")
	cmd.Flags().IntVar(&searchLimit, "limit", 50, "Maximum results")

	// Add global flags
	config.AddGlobalFlags(cmd)

	return cmd
}