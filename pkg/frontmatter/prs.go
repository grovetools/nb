package frontmatter

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// PRsSchemaVersion is the version of the `prs:` ticket↔PR join shape this
// build writes. It is emitted as the sibling `prs_schema_version` key whenever
// a non-empty `prs:` list is serialized (STATE.md D6: every new persisted shape
// carries a version and readers tolerate the old shape).
//
// Reader tolerance rules:
//   - absent/0 → treat as version 1 (the shape below);
//   - a HIGHER version than this build knows → entries still parse, unknown
//     per-entry keys survive in PRRef.Extra, and the declared version is
//     re-emitted unchanged so a newer writer's shape is never downgraded.
const PRsSchemaVersion = 1

// PRRef is one entry of the ticket↔PR join: the edge from a ticket note to a
// pull request opened for the work that ticket describes. The shape is the one
// in concepts/hosted-git-and-prs/forge-provider.md ("The ticket↔PR join").
//
// Nothing writes these automatically in this wave — publish is deferred — so
// every field must tolerate being hand-edited or absent.
type PRRef struct {
	// Repo is the ecosystem repository the PR belongs to (e.g. "flow"), not a
	// forge slug — a ticket's PRs span repos within one plan.
	Repo string `yaml:"repo,omitempty"`
	// Provider names the forge ("github", "forgejo"). Empty means unknown, not
	// GitHub: state refresh skips entries it cannot attribute to a provider.
	Provider string `yaml:"provider,omitempty"`
	// URL is the PR's canonical web URL and the entry's identity.
	URL string `yaml:"url,omitempty"`
	// State mirrors the forge ("open", "closed", "merged", "draft"). Empty
	// renders as unknown and must never render as merged/green (D4).
	State string `yaml:"state,omitempty"`
	// UpdatedAt is when State was last observed, RFC3339.
	UpdatedAt string `yaml:"updated_at,omitempty"`

	// Extra retains any key this build does not know so a newer writer's
	// fields survive a read/write round-trip through an older nb.
	Extra map[string]any `yaml:",inline"`
}

// PRList is the parsed `prs:` value. It exists (rather than a bare []PRRef) so
// a hand-edited value that is not a list of mappings neither fails the parse of
// the whole note nor gets silently deleted on the next write: the original node
// is retained and re-emitted verbatim.
type PRList struct {
	Entries []PRRef

	// raw holds the original node when it could not be decoded as []PRRef.
	// Entries is empty in that case and Build re-emits raw unchanged.
	raw *yaml.Node
}

// UnmarshalYAML decodes the `prs:` value leniently. A value that is not a list
// of mappings is preserved rather than rejected: manual-edit tolerance is a
// deliverable, and failing here would make the entire note unparseable.
func (l *PRList) UnmarshalYAML(node *yaml.Node) error {
	var entries []PRRef
	if err := node.Decode(&entries); err != nil {
		l.Entries = nil
		l.raw = node
		return nil
	}
	for i := range entries {
		if len(entries[i].Extra) == 0 {
			entries[i].Extra = nil
		}
	}
	l.Entries = entries
	l.raw = nil
	return nil
}

// IsZero reports whether there is nothing to serialize.
func (l PRList) IsZero() bool {
	return len(l.Entries) == 0 && l.raw == nil
}

// Unparsed reports whether the `prs:` value was present but not in the expected
// shape. Callers that mutate entries must not do so when this is true — the
// value is being preserved verbatim, not understood.
func (l PRList) Unparsed() bool {
	return l.raw != nil
}

// PRIssue is one validation complaint about a `prs:` entry. Validation is
// advisory: it reports what is malformed without dropping or rewriting
// anything, so a hand-edited note is diagnosed rather than destroyed.
type PRIssue struct {
	Index   int    // position in the list, -1 for list-level issues
	Field   string // the offending key, "" for whole-entry issues
	Message string
}

func (i PRIssue) String() string {
	switch {
	case i.Index < 0:
		return fmt.Sprintf("prs: %s", i.Message)
	case i.Field == "":
		return fmt.Sprintf("prs[%d]: %s", i.Index, i.Message)
	default:
		return fmt.Sprintf("prs[%d].%s: %s", i.Index, i.Field, i.Message)
	}
}

// KnownPRStates are the states this build renders distinctly. An unrecognized
// state is preserved and reported, never coerced — coercing an unknown state
// risks rendering it as merged/green, which D4 forbids.
var KnownPRStates = []string{"open", "closed", "merged", "draft"}

// ValidatePRs checks a parsed `prs:` list and returns advisory issues. An empty
// result means the list is well-formed. It never mutates the list.
func ValidatePRs(l PRList) []PRIssue {
	var issues []PRIssue

	if l.Unparsed() {
		return []PRIssue{{
			Index:   -1,
			Message: "value is not a list of {repo, provider, url, state, updated_at} entries; preserved verbatim and ignored",
		}}
	}

	seen := make(map[string]int, len(l.Entries))
	for i, e := range l.Entries {
		if e.URL == "" {
			issues = append(issues, PRIssue{Index: i, Field: "url", Message: "required (a PR entry is identified by its URL)"})
		} else if prev, dup := seen[e.URL]; dup {
			issues = append(issues, PRIssue{Index: i, Field: "url", Message: fmt.Sprintf("duplicate of entry %d", prev)})
		} else {
			seen[e.URL] = i
		}

		if e.Repo == "" {
			issues = append(issues, PRIssue{Index: i, Field: "repo", Message: "required (which ecosystem repo this PR is against)"})
		}

		if e.State != "" && !isKnownPRState(e.State) {
			issues = append(issues, PRIssue{
				Index: i, Field: "state",
				Message: fmt.Sprintf("unrecognized state %q (known: %s); preserved and rendered as unknown", e.State, strings.Join(KnownPRStates, ", ")),
			})
		}

		if e.UpdatedAt != "" {
			if _, err := ParseTimestamp(e.UpdatedAt); err != nil {
				issues = append(issues, PRIssue{Index: i, Field: "updated_at", Message: fmt.Sprintf("unparseable timestamp %q", e.UpdatedAt)})
			}
		}
	}

	return issues
}

func isKnownPRState(state string) bool {
	s := strings.ToLower(strings.TrimSpace(state))
	for _, known := range KnownPRStates {
		if s == known {
			return true
		}
	}
	return false
}

// FindPRByURL returns the index of the entry with the given URL, or -1. It is
// the lookup the read-only state refresh uses: URL is the entry's identity, so
// refresh updates in place and never appends.
func FindPRByURL(l PRList, url string) int {
	for i, e := range l.Entries {
		if e.URL == url {
			return i
		}
	}
	return -1
}

// renderPRs serializes the `prs:` block (key line included, trailing newline
// included), or "" when there is nothing to write. Entries go through
// yaml.Marshal rather than the hand-rolled formatters used for scalars above:
// entries are nested and carry arbitrary preserved keys, which the scalar
// formatters cannot quote correctly.
func renderPRs(l PRList) string {
	if l.Unparsed() {
		return renderPreservedNode("prs", l.raw)
	}
	if len(l.Entries) == 0 {
		return ""
	}

	out, err := yaml.Marshal(l.Entries)
	if err != nil {
		// Unreachable for this shape; drop the block rather than emit
		// corrupt YAML that would make the note unparseable.
		return ""
	}
	return "prs:\n" + indentYAML(string(out), "  ")
}

// renderPreservedNode re-emits a value nb did not understand under the given
// key, unchanged. Scalars stay inline so a one-line hand edit round-trips as
// one line.
func renderPreservedNode(key string, node *yaml.Node) string {
	out, err := yaml.Marshal(node)
	if err != nil {
		return ""
	}
	body := strings.TrimRight(string(out), "\n")
	if body == "" {
		return ""
	}
	if node.Kind == yaml.ScalarNode && !strings.Contains(body, "\n") {
		return key + ": " + body + "\n"
	}
	return key + ":\n" + indentYAML(string(out), "  ")
}

// indentYAML prefixes every non-empty line of a YAML fragment, normalizing the
// trailing newline so blocks concatenate cleanly.
func indentYAML(s, prefix string) string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = prefix + line
		}
	}
	return strings.Join(lines, "\n") + "\n"
}
