package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	grovelogging "github.com/mattsolo1/grove-core/logging"
	"github.com/mattsolo1/grove-core/version"
	"github.com/spf13/cobra"
)

var versionUlog = grovelogging.NewUnifiedLogger("grove-notebook.cmd.version")

func NewVersionCmd() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Long:  "Display the version, commit, branch, and build information for grove-notebook",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			info := version.GetInfo()

			if jsonOutput {
				jsonData, err := json.MarshalIndent(info, "", "  ")
				if err != nil {
					return fmt.Errorf("failed to marshal version info to JSON: %w", err)
				}
				versionUlog.Info("Version info").
					Field("version", info.Version).
					Field("commit", info.Commit).
					Field("branch", info.Branch).
					Pretty(string(jsonData)).
					PrettyOnly().
					Log(ctx)
			} else {
				versionUlog.Info("Version info").
					Field("version", info.Version).
					Field("commit", info.Commit).
					Field("branch", info.Branch).
					Pretty(info.String()).
					PrettyOnly().
					Log(ctx)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output version information in JSON format")

	return cmd
}