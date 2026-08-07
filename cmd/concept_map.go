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
  nb concept map list --federated
  nb concept map run payments --port 4001
  nb concept map validate payments --file src/model.c4
  nb concept map update payments
  nb concept map attach tuimux:embeddable-panel-system`,
	}

	cmd.AddCommand(newConceptMapNewCmd(svc, workspaceOverride))
	cmd.AddCommand(newConceptMapListCmd(svc, workspaceOverride))
	cmd.AddCommand(newConceptMapRunCmd(svc, workspaceOverride))
	cmd.AddCommand(newConceptMapValidateCmd(svc, workspaceOverride))
	cmd.AddCommand(newConceptMapUpdateCmd(svc, workspaceOverride))
	cmd.AddCommand(newConceptMapAttachCmd(svc, workspaceOverride))
	cmd.AddCommand(newConceptMapDetachCmd(svc, workspaceOverride))

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

func newConceptMapListCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	var jsonOutput bool
	var federated bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List concept maps and the concepts federating detail into them",
		Long: `List every concept that carries a LikeC4 project.

For each map: its workspace-qualified id, its title, how many .c4 files its
own src/ tree holds, and every likec4.config.json include.paths entry. Each
entry is reverse-mapped to the concept that owns it (<workspace>:<concept-id>)
by matching the resolved path against the concept directories nb discovers
across the notebook's workspaces. An entry with nothing behind it is flagged
dead and reported with the same wording 'nb concept map update' warns with.

--federated narrows the listing to maps that actually have include.paths
entries, and lists each entry's contributor .c4 files.

Maps are discovered across every workspace nb knows about — the same universe
'attach'/'detach' default their --map to — so -W only selects the workspace
context, it does not scope the listing.`,
		Example: `  nb concept map list
  nb concept map list --json
  nb concept map list --federated`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := (*svc).GetWorkspaceContext(*workspaceOverride); err != nil {
				return fmt.Errorf("get workspace context: %w", err)
			}
			listings, err := (*svc).ListConceptMapsDetailed(service.ConceptMapListOptions{Federated: federated})
			if err != nil {
				return fmt.Errorf("list concept maps: %w", err)
			}

			if jsonOutput {
				if listings == nil {
					listings = []service.ConceptMapListing{}
				}
				data, err := json.Marshal(listings)
				if err != nil {
					return fmt.Errorf("marshal json: %w", err)
				}
				fmt.Println(string(data))
				return nil
			}

			printConceptMapListings(listings, federated)
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output result as JSON")
	cmd.Flags().BoolVar(&federated, "federated", false, "Only maps with include.paths entries; list contributor files per include")
	return cmd
}

// printConceptMapListings renders the human form of `concept map list`: one
// block per map, with a '!' marker on dead includes and the canonical
// dead-include warnings underneath.
func printConceptMapListings(listings []service.ConceptMapListing, federated bool) {
	if len(listings) == 0 {
		if federated {
			fmt.Println("No federated concept maps found.")
		} else {
			fmt.Println("No concept maps found.")
		}
		return
	}

	fmt.Printf("Concept maps (%d):\n", len(listings))
	for _, listing := range listings {
		fmt.Printf("  - %s\n", conceptMapRef(listing.ConceptMapInfo))
		if listing.Title != "" && listing.Title != listing.ID {
			fmt.Printf("    %s\n", listing.Title)
		}
		fmt.Printf("    src/: %d .c4 file(s)\n", listing.SrcFiles)
		if len(listing.Includes) == 0 {
			fmt.Printf("    include.paths: (none)\n")
			continue
		}
		fmt.Printf("    include.paths (%d):\n", len(listing.Includes))
		for _, include := range listing.Includes {
			marker := " "
			owner := include.Concept
			if owner == "" {
				owner = "(no concept)"
			}
			if include.Dead {
				marker = "!"
				owner += " — dead"
			}
			fmt.Printf("      %s %s -> %s\n", marker, include.Path, owner)
			for _, file := range include.Files {
				fmt.Printf("          %s\n", file)
			}
			if federated && !include.Dead && len(include.Files) == 0 {
				fmt.Printf("          (no .c4 files)\n")
			}
		}
		for _, warning := range listing.Warnings {
			fmt.Printf("    warning %s\n", warning)
		}
	}
}

// conceptMapRef is the workspace-qualified reference for a map, falling back
// to the bare id for concepts nb could not attribute to a workspace.
func conceptMapRef(info service.ConceptMapInfo) string {
	if info.Workspace == "" {
		return info.ID
	}
	return info.Workspace + ":" + info.ID
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
		Long: `Refresh a concept map's generated files.

package.json is rewritten when its generated content changed (e.g. a likec4
pin bump). likec4.config.json is merge-rewritten: only the generated keys
($schema, name, title) are updated and every user-added key (include,
exclude, ...) is preserved. The seed src/*.c4 files are only created when
src/ contains no .c4 files at all — existing map content is never touched;
content refresh is agent-driven, not scaffolded.`,
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
			for _, warning := range scaffold.Warnings {
				fmt.Printf("  warning %s\n", warning)
			}
			fmt.Printf("Content refresh is agent-driven: %s\n", hint)
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output result as JSON")
	return cmd
}

func newConceptMapAttachCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	var mapRef string
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "attach <concept-id|workspace:concept-id>",
		Short: "Include a concept's likec4/ detail files in a map",
		Long: `Attach a concept to a concept map as federated detail.

The concept's likec4/ folder (created when missing) is added to the map's
likec4.config.json include.paths, as a path relative to the config's folder.
The config is merge-rewritten: every other include entry and every user key is
preserved. Attaching twice changes nothing.

Federated files become full project members: they may extend main-model
elements, add relationships to them, and define views — using only the kinds
already declared in the map's src/_spec.c4. The map is validated afterwards so
a bad detail file surfaces immediately.

--map defaults to the only concept map found; with several maps it is
required.`,
		Example: `  nb concept map attach tuimux:embeddable-panel-system
  nb concept map attach embeddable-panel-system --map grove-architecture
  nb concept map attach payments-detail --map payments --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, conceptDir, err := resolveConceptMapAttachTarget(svc, workspaceOverride, mapRef, args[0])
			if err != nil {
				return err
			}
			attachment, err := (*svc).AttachConceptToMap(target.MapDir, conceptDir)
			if err != nil {
				return err
			}
			validation := validateConceptMapDir(target.MapDir)
			printConceptMapAttachment(attachment, validation, target.ID, args[0], "Attached", jsonOutput)
			if validation.Ran && !validation.OK {
				return fmt.Errorf("map '%s' does not validate after attaching '%s' (see the report above)", target.ID, args[0])
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&mapRef, "map", "", "Concept map to attach to (default: the only map found)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output result as JSON")
	return cmd
}

func newConceptMapDetachCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	var mapRef string
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "detach <concept-id|workspace:concept-id>",
		Short: "Stop including a concept's likec4/ detail files in a map",
		Long: `Detach a concept from a concept map.

Only the entry pointing at this concept's likec4/ folder is removed from
include.paths; every other entry and every user key in likec4.config.json is
preserved, and the concept's own likec4/ files are left on disk. Detaching a
concept that was never attached changes nothing.

--map defaults to the only concept map found; with several maps it is
required.`,
		Example: `  nb concept map detach tuimux:embeddable-panel-system
  nb concept map detach embeddable-panel-system --map grove-architecture --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, conceptDir, err := resolveConceptMapAttachTarget(svc, workspaceOverride, mapRef, args[0])
			if err != nil {
				return err
			}
			attachment, err := (*svc).DetachConceptFromMap(target.MapDir, conceptDir)
			if err != nil {
				return err
			}
			printConceptMapAttachment(attachment, nil, target.ID, args[0], "Detached", jsonOutput)
			return nil
		},
	}

	cmd.Flags().StringVar(&mapRef, "map", "", "Concept map to detach from (default: the only map found)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output result as JSON")
	return cmd
}

// resolveConceptMapAttachTarget resolves both sides of an attach/detach: the
// map (explicit --map, else the sole map in the notebook) and the concept
// whose likec4/ detail folder is being (un)federated.
func resolveConceptMapAttachTarget(svc **service.Service, workspaceOverride *string, mapRef, conceptRef string) (*service.ConceptMapInfo, string, error) {
	ctx, err := (*svc).GetWorkspaceContext(*workspaceOverride)
	if err != nil {
		return nil, "", fmt.Errorf("get workspace context: %w", err)
	}

	var target *service.ConceptMapInfo
	if mapRef != "" {
		dir, err := conceptMapDirForRef(svc, ctx, mapRef)
		if err != nil {
			return nil, "", err
		}
		id := mapRef
		if _, unqualified, qualified := strings.Cut(mapRef, ":"); qualified {
			id = unqualified
		}
		target = &service.ConceptMapInfo{
			ConceptInfo: service.ConceptInfo{ID: id, Path: dir},
			MapDir:      dir,
		}
	} else {
		maps, err := (*svc).ListConceptMaps()
		if err != nil {
			return nil, "", fmt.Errorf("list concept maps: %w", err)
		}
		switch len(maps) {
		case 0:
			return nil, "", fmt.Errorf("no concept map found; create one with 'nb concept map new <id>' or pass --map")
		case 1:
			target = &maps[0]
		default:
			refs := make([]string, 0, len(maps))
			for _, m := range maps {
				refs = append(refs, conceptMapRef(m))
			}
			return nil, "", fmt.Errorf("--map is required: %d concept maps exist (%s)", len(maps), strings.Join(refs, ", "))
		}
	}

	conceptDir, err := (*svc).ResolveConceptPath(ctx, conceptRef)
	if err != nil {
		return nil, "", err
	}
	return target, conceptDir, nil
}

// printConceptMapAttachment renders an attach/detach result (verb is the past
// tense used in the human line), including the resulting include.paths.
func printConceptMapAttachment(attachment *service.ConceptMapAttachment, validation *conceptMapValidation, mapID, conceptRef, verb string, jsonOutput bool) {
	if jsonOutput {
		result := struct {
			*service.ConceptMapAttachment
			Map        string                `json:"map"`
			Concept    string                `json:"concept"`
			Validation *conceptMapValidation `json:"validation,omitempty"`
		}{attachment, mapID, conceptRef, validation}
		data, err := json.Marshal(result)
		if err != nil {
			fmt.Fprintf(os.Stderr, "marshal json: %v\n", err)
			return
		}
		fmt.Println(string(data))
		return
	}

	switch {
	case attachment.Changed:
		fmt.Printf("%s concept '%s' %s map '%s'\n", verb, conceptRef, attachPreposition(verb), mapID)
	case verb == "Attached":
		fmt.Printf("No change: concept '%s' is already attached to map '%s'\n", conceptRef, mapID)
	default:
		fmt.Printf("No change: concept '%s' was not attached to map '%s'\n", conceptRef, mapID)
	}
	if attachment.CreatedDetailDir {
		fmt.Printf("  created %s\n", attachment.DetailDir)
	}
	fmt.Printf("  include.paths:\n")
	if len(attachment.IncludePaths) == 0 {
		fmt.Printf("    (none)\n")
	}
	for _, path := range attachment.IncludePaths {
		marker := " "
		if path == attachment.Path {
			marker = "*"
		}
		fmt.Printf("    %s %s\n", marker, path)
	}
	if validation == nil {
		return
	}
	switch {
	case !validation.Ran:
		fmt.Printf("  warning validation skipped: %s\n", validation.Skipped)
	case validation.OK:
		fmt.Printf("  map validates\n")
	default:
		fmt.Fprintf(os.Stderr, "map validation failed:\n%s\n", validation.Output)
	}
}

func attachPreposition(verb string) string {
	if verb == "Attached" {
		return "to"
	}
	return "from"
}

// conceptMapValidation is the outcome of the post-attach validation pass.
type conceptMapValidation struct {
	Ran     bool   `json:"ran"`
	OK      bool   `json:"ok"`
	Output  string `json:"output,omitempty"`
	Skipped string `json:"skipped,omitempty"`
}

// validateConceptMapDir runs likec4 validate over the map and captures the
// report instead of streaming it, so attach can decide what to surface. A
// missing npx is reported as skipped rather than failing the attach: the
// config edit already succeeded.
func validateConceptMapDir(dir string) *conceptMapValidation {
	npxPath, err := exec.LookPath("npx")
	if err != nil {
		return &conceptMapValidation{Skipped: "'npx' not found in PATH (the likec4 backend requires Node.js >= 22)"}
	}
	output, err := exec.Command(npxPath, "likec4", "validate", "--json", "--no-layout", dir).CombinedOutput()
	return &conceptMapValidation{Ran: true, OK: err == nil, Output: strings.TrimSpace(string(output))}
}

// resolveConceptMapDir resolves a concept reference exactly like
// `nb concept path` and verifies the concept actually contains a LikeC4
// project scaffold.
func resolveConceptMapDir(svc **service.Service, workspaceOverride *string, ref string) (string, error) {
	ctx, err := (*svc).GetWorkspaceContext(*workspaceOverride)
	if err != nil {
		return "", fmt.Errorf("get workspace context: %w", err)
	}
	return conceptMapDirForRef(svc, ctx, ref)
}

// conceptMapDirForRef is resolveConceptMapDir over an already-resolved
// workspace context.
func conceptMapDirForRef(svc **service.Service, ctx *service.WorkspaceContext, ref string) (string, error) {
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
