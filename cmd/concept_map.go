package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/grovetools/nb/pkg/service"
)

func newConceptMapCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "map",
		Short: "Manage architecture maps stored as concepts",
		Long: `Create and operate architecture maps ("concept maps") stored in the notebook.

A map IS a concept: 'new' creates a concept through the normal concept
machinery, then scaffolds a LikeC4 project inside the concept directory.
LikeC4 is the only supported backend for now; running or validating a map
requires Node.js >= 22 (for npx).`,
		Example: `  nb concept map new payments --title "Payments Landscape"
  nb concept map run payments --port 4001
  nb concept map validate payments --file src/model.c4
  nb concept map update payments`,
	}

	cmd.AddCommand(newConceptMapNewCmd(svc, workspaceOverride))
	cmd.AddCommand(newConceptMapRunCmd(svc, workspaceOverride))
	cmd.AddCommand(newConceptMapValidateCmd(svc, workspaceOverride))
	cmd.AddCommand(newConceptMapUpdateCmd(svc, workspaceOverride))

	return cmd
}

func newConceptMapNewCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	var title string
	var backend string
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "new <id>",
		Short: "Create a concept map (a concept with a LikeC4 project inside)",
		Long: `Create a new concept and scaffold a LikeC4 project inside its directory.

The id becomes both the concept directory name and the LikeC4 project name,
so it must not be "default" and cannot contain '.', '@', or '#'.`,
		Example: `  nb concept map new payments
  nb concept map new payments --title "Payments Landscape" --json
  nb concept map new payments --backend likec4`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			if backend != service.ConceptMapBackendLikeC4 {
				return fmt.Errorf("unsupported backend %q (only %q is supported)", backend, service.ConceptMapBackendLikeC4)
			}
			if err := service.ValidateConceptMapID(id); err != nil {
				return err
			}
			mapTitle := title
			if mapTitle == "" {
				mapTitle = id
			}

			ctx, err := (*svc).GetWorkspaceContext(*workspaceOverride)
			if err != nil {
				return fmt.Errorf("get workspace context: %w", err)
			}

			note, err := (*svc).CreateConcept(ctx, mapTitle, service.WithConceptID(id))
			if err != nil {
				return err
			}

			scaffold, err := (*svc).ScaffoldConceptMap(note.Path, id, mapTitle)
			if err != nil {
				return err
			}

			if jsonOutput {
				workspaceName := ""
				if ctx.NotebookContextWorkspace != nil {
					workspaceName = ctx.NotebookContextWorkspace.Name
				}
				result := struct {
					service.ConceptInfo
					MapDir string `json:"mapDir"`
				}{
					ConceptInfo: service.ConceptInfo{
						ID:        id,
						Title:     mapTitle,
						Path:      note.Path,
						Workspace: workspaceName,
					},
					MapDir: scaffold.Dir,
				}
				data, err := json.Marshal(result)
				if err != nil {
					return fmt.Errorf("marshal json: %w", err)
				}
				fmt.Println(string(data))
			} else {
				fmt.Printf("Created concept map: %s\n", note.Path)
				fmt.Printf("  - concept-manifest.yml\n")
				fmt.Printf("  - overview.md\n")
				for _, file := range scaffold.Wrote {
					fmt.Printf("  - %s\n", file)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&title, "title", "", "Map title; defaults to the id")
	cmd.Flags().StringVar(&backend, "backend", service.ConceptMapBackendLikeC4, "Map backend (only likec4 is supported)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output result as JSON")
	return cmd
}

func newConceptMapRunCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	var port int

	cmd := &cobra.Command{
		Use:   "run <concept-id|workspace:concept-id>",
		Short: "Start the LikeC4 dev server for a concept map",
		Long: `Resolve the concept map's directory and exec 'npx likec4 start' on it,
inheriting stdio. Requires Node.js >= 22 so 'npx' is available on PATH.`,
		Example: `  nb concept map run payments
  nb concept map run payments --port 4001
  nb concept map run core:workspace-model`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := resolveConceptMapDir(svc, workspaceOverride, args[0])
			if err != nil {
				return err
			}
			npxArgs := []string{"likec4", "start", dir}
			if port > 0 {
				npxArgs = append(npxArgs, "--port", strconv.Itoa(port))
			}
			return runNpx(npxArgs)
		},
	}

	cmd.Flags().IntVar(&port, "port", 0, "Port for the LikeC4 dev server (default: likec4's own default)")
	return cmd
}

func newConceptMapValidateCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	var files []string

	cmd := &cobra.Command{
		Use:   "validate <concept-id|workspace:concept-id>",
		Short: "Validate a concept map's LikeC4 model (JSON output)",
		Long: `Exec 'npx likec4 validate --json --no-layout' on the concept map's directory,
streaming the JSON report to stdout and propagating likec4's exit code.
This is the agent feedback loop for map edits.`,
		Example: `  nb concept map validate payments
  nb concept map validate payments --file src/model.c4
  nb concept map validate payments --file src/model.c4 --file src/_spec.c4`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := resolveConceptMapDir(svc, workspaceOverride, args[0])
			if err != nil {
				return err
			}
			npxArgs := []string{"likec4", "validate", "--json", "--no-layout"}
			for _, file := range files {
				npxArgs = append(npxArgs, "--file", file)
			}
			npxArgs = append(npxArgs, dir)
			return runNpx(npxArgs)
		},
	}

	cmd.Flags().StringArrayVar(&files, "file", nil, "Validate only this file (repeatable, relative to the map dir)")
	return cmd
}

func newConceptMapUpdateCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "update <concept-id|workspace:concept-id>",
		Short: "Re-scaffold a concept map's generated files (idempotent)",
		Long: `Rewrite likec4.config.json and package.json when their generated content
changed (e.g. a likec4 pin bump) and create any missing scaffold files.
Existing src/*.c4 files are never touched — map content refresh is
agent-driven, not scaffolded.`,
		Example: `  nb concept map update payments
  nb concept map update payments --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref := args[0]
			id := ref
			if _, unqualified, qualified := strings.Cut(ref, ":"); qualified {
				id = unqualified
			}
			ctx, err := (*svc).GetWorkspaceContext(*workspaceOverride)
			if err != nil {
				return fmt.Errorf("get workspace context: %w", err)
			}
			dir, err := (*svc).ResolveConceptPath(ctx, ref)
			if err != nil {
				return err
			}

			scaffold, err := (*svc).ScaffoldConceptMap(dir, id, "")
			if err != nil {
				return err
			}

			hint := fmt.Sprintf("flow plan init --recipe grove/concept-map --recipe-vars map=%s", id)
			if jsonOutput {
				result := struct {
					*service.ConceptMapScaffold
					Hint string `json:"hint"`
				}{scaffold, hint}
				data, err := json.Marshal(result)
				if err != nil {
					return fmt.Errorf("marshal json: %w", err)
				}
				fmt.Println(string(data))
				return nil
			}

			fmt.Printf("Updated concept map: %s\n", scaffold.Dir)
			for _, file := range scaffold.Wrote {
				fmt.Printf("  wrote   %s\n", file)
			}
			for _, file := range scaffold.Updated {
				fmt.Printf("  updated %s\n", file)
			}
			for _, file := range scaffold.Skipped {
				fmt.Printf("  kept    %s\n", file)
			}
			fmt.Printf("Content refresh is agent-driven: %s\n", hint)
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output result as JSON")
	return cmd
}

// resolveConceptMapDir resolves a concept reference exactly like
// `nb concept path` and verifies the concept actually contains a LikeC4
// project scaffold.
func resolveConceptMapDir(svc **service.Service, workspaceOverride *string, ref string) (string, error) {
	ctx, err := (*svc).GetWorkspaceContext(*workspaceOverride)
	if err != nil {
		return "", fmt.Errorf("get workspace context: %w", err)
	}
	dir, err := (*svc).ResolveConceptPath(ctx, ref)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(dir, "likec4.config.json")); err != nil {
		return "", fmt.Errorf("concept '%s' has no LikeC4 project (missing likec4.config.json); run 'nb concept map update %s' to scaffold one", ref, ref)
	}
	return dir, nil
}

// runNpx probes for npx on PATH, then runs it with inherited stdio and
// propagates the child's exit code.
func runNpx(args []string) error {
	npxPath, err := exec.LookPath("npx")
	if err != nil {
		return fmt.Errorf("'npx' not found in PATH: the likec4 backend requires Node.js >= 22 (which provides npx); install it from https://nodejs.org and re-run")
	}
	child := exec.Command(npxPath, args...)
	child.Stdin = os.Stdin
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	if err := child.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() >= 0 {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("npx %s failed: %w", strings.Join(args[:2], " "), err)
	}
	return nil
}
