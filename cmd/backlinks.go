package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/grovetools/nb/pkg/index"
	"github.com/grovetools/nb/pkg/service"
)

// buildVaultIndex builds the Phase-1 vault index over the current workspace
// context: all content dirs (notes/plans/chats) plus the concepts dir.
func buildVaultIndex(s *service.Service, workspaceOverride string) (*index.Index, error) {
	ctx, err := s.GetWorkspaceContext(workspaceOverride)
	if err != nil {
		return nil, fmt.Errorf("get workspace context: %w", err)
	}
	node := ctx.NotebookContextWorkspace
	locator := s.GetNotebookLocator()

	var roots []index.Root
	dirs, err := locator.GetAllContentDirs(node)
	if err != nil {
		return nil, fmt.Errorf("get content dirs: %w", err)
	}
	for _, d := range dirs {
		roots = append(roots, index.Root{Dir: d.Path, Workspace: node.Name})
	}
	if conceptsDir, err := locator.GetNotesDir(node, "concepts"); err == nil {
		roots = append(roots, index.Root{Dir: conceptsDir, Workspace: node.Name})
	}

	ix := index.New()
	if err := ix.Build(roots); err != nil {
		return nil, fmt.Errorf("build vault index: %w", err)
	}
	return ix, nil
}

// resolveNoteArg turns a CLI note argument (path, stem, id, alias, or title)
// into a single indexed doc.
func resolveNoteArg(ix *index.Index, arg string) (*index.Doc, error) {
	// Existing file path (relative or absolute) wins.
	if abs, err := filepath.Abs(arg); err == nil {
		if _, statErr := os.Stat(abs); statErr == nil {
			if d, ok := ix.Doc(abs); ok {
				return d, nil
			}
			return nil, fmt.Errorf("file exists but is not in the vault index: %s", abs)
		}
	}
	candidates := ix.Resolve(arg)
	switch len(candidates) {
	case 0:
		return nil, fmt.Errorf("no note found for %q", arg)
	case 1:
		return candidates[0], nil
	default:
		var b strings.Builder
		fmt.Fprintf(&b, "%q is ambiguous, matches:\n", arg)
		for _, c := range candidates {
			fmt.Fprintf(&b, "  %s\n", c.Path)
		}
		return nil, fmt.Errorf("%s", strings.TrimRight(b.String(), "\n"))
	}
}

func printLinksJSON(links []index.Link) error {
	if links == nil {
		links = []index.Link{}
	}
	data, err := json.Marshal(links)
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

func NewBacklinksCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "backlinks <note>",
		Short: "List notes that link to the given note",
		Long: `List all vault links pointing at the given note.

The note may be given as a file path, filename stem, frontmatter id, alias,
or exact title.

Examples:
  nb backlinks my-note            # By stem
  nb backlinks plans/foo/note.md  # By path
  nb backlinks my-note --json     # Machine-readable output`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ix, err := buildVaultIndex(*svc, *workspaceOverride)
			if err != nil {
				return err
			}
			doc, err := resolveNoteArg(ix, args[0])
			if err != nil {
				return err
			}
			backlinks := ix.Backlinks(doc.Path)

			if jsonOutput {
				return printLinksJSON(backlinks)
			}
			if len(backlinks) == 0 {
				fmt.Printf("No backlinks to %s\n", doc.Path)
				return nil
			}
			fmt.Printf("%d backlink(s) to %s:\n", len(backlinks), doc.Path)
			for _, l := range backlinks {
				fmt.Printf("  %s:%d  [[%s]]\n", l.SourcePath, l.Line, l.RawTarget)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output result as JSON")
	return cmd
}

func NewLinksCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	var (
		jsonOutput bool
		unresolved bool
	)

	cmd := &cobra.Command{
		Use:   "links [note]",
		Short: "List outgoing links of a note, or unresolved links",
		Long: `List the outgoing vault links of a note.

With --unresolved and no note, list every unresolved wikilink in the vault;
with --unresolved and a note, list only that note's unresolved links.

Examples:
  nb links my-note              # Outgoing links
  nb links my-note --unresolved # Broken links in one note
  nb links --unresolved         # All broken links in the vault`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && !unresolved {
				return fmt.Errorf("a note argument is required unless --unresolved is given")
			}
			ix, err := buildVaultIndex(*svc, *workspaceOverride)
			if err != nil {
				return err
			}

			var links []index.Link
			var header string
			switch {
			case len(args) == 0:
				links = ix.Unresolved()
				header = fmt.Sprintf("%d unresolved link(s) in the vault:", len(links))
			default:
				doc, err := resolveNoteArg(ix, args[0])
				if err != nil {
					return err
				}
				for _, l := range doc.Links {
					if unresolved && l.ResolvedPath != "" {
						continue
					}
					links = append(links, l)
				}
				header = fmt.Sprintf("%d outgoing link(s) from %s:", len(links), doc.Path)
				if unresolved {
					header = fmt.Sprintf("%d unresolved link(s) in %s:", len(links), doc.Path)
				}
			}

			if jsonOutput {
				return printLinksJSON(links)
			}
			if len(links) == 0 {
				fmt.Println(strings.Replace(header, ":", ".", 1))
				return nil
			}
			fmt.Println(header)
			for _, l := range links {
				target := l.ResolvedPath
				if target == "" {
					target = "(unresolved)"
				}
				fmt.Printf("  %s:%d  [[%s]] -> %s\n", l.SourcePath, l.Line, l.RawTarget, target)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output result as JSON")
	cmd.Flags().BoolVar(&unresolved, "unresolved", false, "Show only unresolved links (vault-wide when no note is given)")
	return cmd
}

func NewTagsCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "tags",
		Short: "List all tags in the vault with note counts",
		Long: `List every tag in the vault (frontmatter, path, and inline #tags)
with the number of notes carrying it.

Examples:
  nb tags
  nb tags --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ix, err := buildVaultIndex(*svc, *workspaceOverride)
			if err != nil {
				return err
			}
			tags := ix.Tags()

			if jsonOutput {
				data, err := json.Marshal(tags)
				if err != nil {
					return fmt.Errorf("marshal json: %w", err)
				}
				fmt.Println(string(data))
				return nil
			}

			if len(tags) == 0 {
				fmt.Println("No tags found.")
				return nil
			}
			names := make([]string, 0, len(tags))
			for name := range tags {
				names = append(names, name)
			}
			sort.Slice(names, func(i, j int) bool {
				if tags[names[i]] != tags[names[j]] {
					return tags[names[i]] > tags[names[j]]
				}
				return names[i] < names[j]
			})
			for _, name := range names {
				fmt.Printf("%5d  #%s\n", tags[name], name)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output result as JSON")
	return cmd
}
