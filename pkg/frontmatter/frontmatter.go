package frontmatter

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var frontmatterPattern = regexp.MustCompile(`(?s)^---\n(.*?)\n---\n(.*)`)

// RemoteMetadata represents sync metadata from remote sources (GitHub, etc.)
type RemoteMetadata struct {
	Provider  string   `yaml:"provider,omitempty"`
	ID        string   `yaml:"id,omitempty"`
	URL       string   `yaml:"url,omitempty"`
	State     string   `yaml:"state,omitempty"`
	UpdatedAt string   `yaml:"updated_at,omitempty"`
	Labels    []string `yaml:"labels,flow,omitempty"`
	Assignees []string `yaml:"assignees,flow,omitempty"`
	Milestone string   `yaml:"milestone,omitempty"`
}

// Frontmatter represents the structured metadata at the beginning of a note
type Frontmatter struct {
	ID         string   `yaml:"id"`
	Title      string   `yaml:"title"`
	Type       string   `yaml:"type,omitempty"` // Note type (chat, interactive_agent, etc.)
	Aliases    []string `yaml:"aliases,flow"`
	Tags       []string `yaml:"tags,flow"`
	Repository string   `yaml:"repository,omitempty"`
	Branch     string   `yaml:"branch,omitempty"`
	Worktree   string   `yaml:"worktree,omitempty"`
	Created    string   `yaml:"created"`
	Modified   string   `yaml:"modified"`
	Started    string   `yaml:"started,omitempty"`  // For LLM notes
	PlanRef    string   `yaml:"plan_ref,omitempty"` // Reference to associated plan (slug form: plans/<planName>)
	PlanJob    string   `yaml:"plan_job,omitempty"` // Per-job linkage: the promoted job's filename (e.g. 01-foo.md)
	Priority   string   `yaml:"priority,omitempty"` // p0 (most critical) .. p3, empty = none
	Name       string   `yaml:"name,omitempty"`     // Canonical name when the filename is generic (e.g. skills/<name>/SKILL.md)

	// Remote sync metadata
	Remote *RemoteMetadata `yaml:"remote,omitempty"`

	// The ticket↔PR join: the pull requests opened for this ticket's work,
	// plus the version of that shape (see prs.go). Nothing writes these
	// automatically yet — publish is deferred — so they exist to be read,
	// hand-edited, and refreshed.
	PRs              PRList `yaml:"prs,omitempty"`
	PRsSchemaVersion int    `yaml:"prs_schema_version,omitempty"`

	// Blog-specific fields
	Description string `yaml:"description,omitempty"`
	PublishDate string `yaml:"publishDate,omitempty"`
	UpdatedDate string `yaml:"updatedDate,omitempty"`
	Draft       bool   `yaml:"draft,omitempty"`
	Featured    bool   `yaml:"featured,omitempty"`

	// Extra carries every frontmatter key this struct has no field for
	// (yaml.v3 collects unmatched keys into the `,inline` map). Before it
	// existed, any Parse→Build cycle — move, copy, internal
	// update-frontmatter — silently stripped producer-owned keys like
	// `pomodoro_block_id`, flow's `status` or grove-gtd's namespaced `gtd:`
	// block, so no external producer could trust nb with structured metadata.
	// Build re-emits these keys after the known fields, so the round-trip is
	// lossless AND deterministic.
	//
	// The values are raw yaml.Node, not decoded Go values, so the passthrough
	// is VERBATIM. Decoding to interface{} would quietly rewrite the data nb
	// is only supposed to be carrying: yaml.v3 resolves an unquoted
	// YYYY-MM-DD to time.Time, so a plugin's `defer: 2026-08-10` came back as
	// an RFC3339 stamp (or worse) on the next unrelated `nb move`. Nodes also
	// preserve the author's key order and flow/block style, so a rewrite of
	// one field never churns the spelling of somebody else's block.
	//
	// nb assigns no meaning to anything in here. Use ExtraValue/ExtraString
	// to read and SetExtra to write rather than touching the nodes directly.
	Extra map[string]yaml.Node `yaml:",inline"`
}

// ExtraValue decodes the extension key into a plain Go value, or returns nil
// if the key is absent (or holds something undecodable). Callers that need the
// raw spelling should read fm.Extra directly — this is the convenience path
// for the handful of places nb inspects an extension key it does not own.
func (fm *Frontmatter) ExtraValue(key string) any {
	node, ok := fm.Extra[key]
	if !ok {
		return nil
	}
	var v any
	if err := node.Decode(&v); err != nil {
		return nil
	}
	return v
}

// ExtraString reads an extension key expected to hold a string scalar. The
// bool reports whether the key was present AND string-shaped, so callers can
// tell "no key" from "a key holding a map".
func (fm *Frontmatter) ExtraString(key string) (string, bool) {
	node, ok := fm.Extra[key]
	if !ok {
		return "", false
	}
	var s string
	if err := node.Decode(&s); err != nil {
		return "", false
	}
	return s, true
}

// SetExtra stores value under an extension key, encoding it to a node. It
// rejects keys that name a typed field above: those are owned by the struct,
// and letting one land in Extra as well would emit the key twice and make the
// note unparseable-in-practice. Values that came from a file are always
// encodable; an unencodable programmatic value (a channel, a func) errors
// rather than corrupting the note.
func (fm *Frontmatter) SetExtra(key string, value any) error {
	if knownFieldNames[key] {
		return fmt.Errorf("frontmatter key %q is a typed field, not an extension key", key)
	}
	var node yaml.Node
	if err := node.Encode(value); err != nil {
		return fmt.Errorf("encode frontmatter key %q: %w", key, err)
	}
	if fm.Extra == nil {
		fm.Extra = map[string]yaml.Node{}
	}
	fm.Extra[key] = node
	return nil
}

// IdempotencyKeyField is the frontmatter key `nb new --idempotency-key`
// stores its key under. It lives in Extra (nb has no semantics for it beyond
// the create-time duplicate scan), which also means moves and copies carry it
// along like any other producer field.
const IdempotencyKeyField = "idempotency_key"

// nbOwnedFields are the frontmatter keys nb itself is authoritative for.
// Producer frontmatter (via --frontmatter-file) never overrides them: id and
// created are the note's identity, modified is stamped by nb on every write,
// type is derived from the placement directory (a frontmatter `type:` claim
// that disagrees with the directory would corrupt the index), and remote is
// owned by the sync system.
var nbOwnedFields = map[string]bool{
	"id":       true,
	"created":  true,
	"modified": true,
	"type":     true,
	"remote":   true,
}

// knownFieldNames are the yaml keys that map to explicit struct fields above.
// Build uses this to refuse to emit an Extra entry that would duplicate a
// known key (duplicate YAML keys make the note unparseable-in-practice), and
// ApplyProducerFields uses it to route producer values into typed fields
// instead of the extension map.
var knownFieldNames = map[string]bool{
	"id": true, "title": true, "type": true, "aliases": true, "tags": true,
	"repository": true, "branch": true, "worktree": true, "created": true,
	"modified": true, "started": true, "plan_ref": true, "plan_job": true,
	"priority": true, "name": true, "remote": true, "prs": true,
	"prs_schema_version": true, "description": true, "publishDate": true,
	"updatedDate": true, "draft": true, "featured": true,
}

// producerKeyPattern is the shape a producer-supplied extension key must have.
// It is deliberately conservative: a leading letter and word-ish characters,
// so keys can never smuggle YAML syntax and namespaced fields (`pomodoro_*`,
// `hn_*`) all pass naturally.
var producerKeyPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]*$`)

// Parse extracts frontmatter from content and returns the parsed data and body
func Parse(content string) (*Frontmatter, string, error) {
	matches := frontmatterPattern.FindStringSubmatch(content)
	if len(matches) != 3 {
		// No frontmatter found
		return nil, content, nil
	}

	frontmatterStr := matches[1]
	bodyContent := matches[2]

	var fm Frontmatter
	if err := yaml.Unmarshal([]byte(frontmatterStr), &fm); err != nil {
		return nil, content, fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	// Ensure arrays are never nil
	if fm.Aliases == nil {
		fm.Aliases = []string{}
	}
	if fm.Tags == nil {
		fm.Tags = []string{}
	}
	// Normalize an empty catch-all to nil so a note with no unknown keys
	// compares equal to a hand-built Frontmatter.
	if len(fm.Extra) == 0 {
		fm.Extra = nil
	}

	return &fm, bodyContent, nil
}

// UpdateField sets a single named frontmatter field to value, used by the
// `nb internal update-frontmatter` command. An empty value CLEARS the link
// fields (plan_ref, plan_job) — flow's demote path relies on this — while every
// other supported field rejects an empty value so it can't be blanked by
// accident. Unsupported field names return an error.
func UpdateField(fm *Frontmatter, field, value string) error {
	switch field {
	case "plan_ref":
		fm.PlanRef = value // empty clears
	case "plan_job":
		fm.PlanJob = value // empty clears
	case "title":
		if value == "" {
			return fmt.Errorf("--value is required for field %q", field)
		}
		fm.Title = value
	case "repository":
		if value == "" {
			return fmt.Errorf("--value is required for field %q", field)
		}
		fm.Repository = value
	case "branch":
		if value == "" {
			return fmt.Errorf("--value is required for field %q", field)
		}
		fm.Branch = value
	case "worktree":
		if value == "" {
			return fmt.Errorf("--value is required for field %q", field)
		}
		fm.Worktree = value
	default:
		return fmt.Errorf("unsupported field: %s", field)
	}
	return nil
}

// Build creates the YAML frontmatter string from a Frontmatter struct
func Build(fm *Frontmatter) string {
	var sb strings.Builder

	sb.WriteString("---\n")

	// Always include these fields in a consistent order
	sb.WriteString(fmt.Sprintf("id: %s\n", formatYAMLValue(fm.ID)))
	sb.WriteString(fmt.Sprintf("title: %s\n", formatYAMLValue(fm.Title)))
	if fm.Type != "" {
		sb.WriteString(fmt.Sprintf("type: %s\n", formatYAMLValue(fm.Type)))
	}
	sb.WriteString(fmt.Sprintf("aliases: %s\n", formatYAMLArray(fm.Aliases)))
	sb.WriteString(fmt.Sprintf("tags: %s\n", formatYAMLArray(fm.Tags)))

	// Optional fields
	if fm.Repository != "" {
		sb.WriteString(fmt.Sprintf("repository: %s\n", formatYAMLValue(fm.Repository)))
	}
	if fm.Branch != "" {
		sb.WriteString(fmt.Sprintf("branch: %s\n", formatYAMLValue(fm.Branch)))
	}
	if fm.Worktree != "" {
		sb.WriteString(fmt.Sprintf("worktree: %s\n", formatYAMLValue(fm.Worktree)))
	}

	// Timestamps
	sb.WriteString(fmt.Sprintf("created: %s\n", fm.Created))
	sb.WriteString(fmt.Sprintf("modified: %s\n", fm.Modified))

	// Special fields
	if fm.Started != "" {
		sb.WriteString(fmt.Sprintf("started: %s\n", formatYAMLValue(fm.Started)))
	}
	if fm.PlanRef != "" {
		sb.WriteString(fmt.Sprintf("plan_ref: %s\n", formatYAMLValue(fm.PlanRef)))
	}
	if fm.PlanJob != "" {
		sb.WriteString(fmt.Sprintf("plan_job: %s\n", formatYAMLValue(fm.PlanJob)))
	}
	if fm.Priority != "" {
		sb.WriteString(fmt.Sprintf("priority: %s\n", formatYAMLValue(fm.Priority)))
	}
	// name was parsed into the struct but never re-emitted, so every
	// Parse→Build cycle stripped it — the same silent-loss bug as the
	// unknown-key drop that Extra fixes, just for a field nb itself declares.
	if fm.Name != "" {
		sb.WriteString(fmt.Sprintf("name: %s\n", formatYAMLValue(fm.Name)))
	}

	// Remote sync metadata
	if fm.Remote != nil {
		sb.WriteString("remote:\n")
		if fm.Remote.Provider != "" {
			sb.WriteString(fmt.Sprintf("  provider: %s\n", formatYAMLValue(fm.Remote.Provider)))
		}
		if fm.Remote.ID != "" {
			sb.WriteString(fmt.Sprintf("  id: %s\n", formatYAMLValue(fm.Remote.ID)))
		}
		if fm.Remote.URL != "" {
			sb.WriteString(fmt.Sprintf("  url: %s\n", formatYAMLValue(fm.Remote.URL)))
		}
		if fm.Remote.State != "" {
			sb.WriteString(fmt.Sprintf("  state: %s\n", formatYAMLValue(fm.Remote.State)))
		}
		if fm.Remote.UpdatedAt != "" {
			sb.WriteString(fmt.Sprintf("  updated_at: %s\n", formatYAMLValue(fm.Remote.UpdatedAt)))
		}
		if len(fm.Remote.Labels) > 0 {
			sb.WriteString(fmt.Sprintf("  labels: %s\n", formatYAMLArray(fm.Remote.Labels)))
		}
		if len(fm.Remote.Assignees) > 0 {
			sb.WriteString(fmt.Sprintf("  assignees: %s\n", formatYAMLArray(fm.Remote.Assignees)))
		}
		if fm.Remote.Milestone != "" {
			sb.WriteString(fmt.Sprintf("  milestone: %s\n", formatYAMLValue(fm.Remote.Milestone)))
		}
	}

	// The ticket↔PR join. The version key precedes the list so a reader sees
	// which shape follows. A version already on the note is re-emitted as-is
	// (never downgraded); an unversioned list is stamped with the version this
	// build understands. A value nb could not parse is passed through WITHOUT
	// a version — stamping one would assert a shape that was never verified.
	if prsBlock := renderPRs(fm.PRs); prsBlock != "" {
		version := fm.PRsSchemaVersion
		if version == 0 && !fm.PRs.Unparsed() {
			version = PRsSchemaVersion
		}
		if version != 0 {
			sb.WriteString(fmt.Sprintf("prs_schema_version: %d\n", version))
		}
		sb.WriteString(prsBlock)
	}

	// Blog-specific fields
	if fm.Description != "" {
		sb.WriteString(fmt.Sprintf("description: %s\n", formatYAMLValue(fm.Description)))
	}
	if fm.PublishDate != "" {
		sb.WriteString(fmt.Sprintf("publishDate: %s\n", formatYAMLValue(fm.PublishDate)))
	}
	if fm.UpdatedDate != "" {
		sb.WriteString(fmt.Sprintf("updatedDate: %s\n", formatYAMLValue(fm.UpdatedDate)))
	}
	if fm.Draft {
		sb.WriteString("draft: true\n")
	}
	if fm.Featured {
		sb.WriteString("featured: true\n")
	}

	// Extension keys come last, sorted, so the known-field prefix of existing
	// notes never churns and repeated Build calls are byte-identical. Keys
	// that collide with a known field are skipped: the typed field already
	// owns that line, and a duplicate YAML key would corrupt the note.
	if len(fm.Extra) > 0 {
		keys := make([]string, 0, len(fm.Extra))
		for k := range fm.Extra {
			if knownFieldNames[k] {
				continue
			}
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			sb.WriteString(marshalExtraEntry(k, fm.Extra[k]))
		}
	}

	sb.WriteString("---")

	return sb.String()
}

// marshalExtraEntry renders one extension key as YAML by re-encoding the node
// the file was parsed into. It goes through the real YAML encoder (2-space
// indent, matching the rest of the codebase) rather than the hand-rolled
// formatYAMLValue, because extension values can be numbers, booleans, lists or
// nested maps — not just strings — and because the node carries its own
// scalar style, so the value is emitted with the spelling it arrived with.
//
// The key is wrapped in a one-entry mapping node rather than a Go map so that
// nothing on this path re-resolves the value's type.
func marshalExtraEntry(key string, value yaml.Node) string {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	entry := yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Value: key},
		&value,
	}}
	if err := enc.Encode(&entry); err != nil {
		// A malformed node (only reachable by building one by hand rather
		// than through Parse or SetExtra) is dropped rather than corrupting
		// the note file.
		return ""
	}
	_ = enc.Close()
	return buf.String()
}

// BuildContent combines frontmatter and body content into a complete document
func BuildContent(fm *Frontmatter, bodyContent string) string {
	frontmatterStr := Build(fm)

	// Ensure proper spacing between frontmatter and body
	if !strings.HasPrefix(bodyContent, "\n") {
		return frontmatterStr + "\n\n" + bodyContent
	}
	return frontmatterStr + "\n" + bodyContent
}

// legacyTimestampFormat is the historical timezone-less frontmatter format.
// Existing files keep it forever (no mass migration); it remains parseable.
const legacyTimestampFormat = "2006-01-02 15:04:05"

// FormatTimestamp formats a time.Time into the standard frontmatter timestamp
// format. New writes are RFC3339 in UTC; legacy timezone-less values in
// existing files are only re-emitted when nb already rewrites a note's
// frontmatter for other reasons.
func FormatTimestamp(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// ParseTimestamp parses a frontmatter timestamp string into time.Time.
// It accepts both the current RFC3339/UTC format and the legacy
// timezone-less format (dual-read forever, per the sync protocol).
func ParseTimestamp(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Parse(legacyTimestampFormat, s)
}

// formatYAMLArray formats a string slice as a YAML flow-style array
func formatYAMLArray(items []string) string {
	if len(items) == 0 {
		return "[]"
	}

	quotedItems := make([]string, len(items))
	for i, item := range items {
		if needsQuoting(item) {
			quotedItems[i] = fmt.Sprintf("%q", item)
		} else {
			quotedItems[i] = item
		}
	}

	return fmt.Sprintf("[%s]", strings.Join(quotedItems, ", "))
}

// formatYAMLValue quotes a string value if it contains YAML-special characters
func formatYAMLValue(s string) string {
	if needsQuoting(s) {
		return fmt.Sprintf("%q", s)
	}
	return s
}

// needsQuoting checks if a string needs to be quoted in YAML
func needsQuoting(s string) bool {
	if strings.ContainsAny(s, ",:[]{}\"'#") {
		return true
	}
	// Also check for leading/trailing whitespace
	if len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		return true
	}
	return false
}

// ExtractPathTags generates tags from a note type path (e.g., "issues/bugs" -> ["issues", "bugs"])
func ExtractPathTags(noteType string) []string {
	if noteType == "" {
		return []string{}
	}

	parts := strings.Split(noteType, "/")
	tags := []string{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			tags = append(tags, part)
		}
	}
	return tags
}

// ProducerFields is the contents of a producer's --frontmatter-file, held as
// raw yaml nodes for the same reason Frontmatter.Extra is: decoding to
// interface{} on the way in would coerce the producer's values (unquoted dates
// to time.Time, most notably) before nb ever writes them, so a plugin could
// not round-trip its own metadata through the structured seam.
type ProducerFields map[string]yaml.Node

// LoadProducerFields reads a producer frontmatter file (--frontmatter-file)
// and returns its top-level fields. The file may be JSON or YAML — JSON is a
// subset of YAML, so one unmarshal handles both — but its root must be a
// mapping; anything else is a malformed producer file, not a note.
func LoadProducerFields(path string) (ProducerFields, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read frontmatter file: %w", err)
	}
	var fields ProducerFields
	if err := yaml.Unmarshal(data, &fields); err != nil {
		return nil, fmt.Errorf("parse frontmatter file %s (JSON or YAML mapping expected): %w", path, err)
	}
	return fields, nil
}

// NewProducerFields builds ProducerFields from plain Go values, for in-process
// callers that have a map rather than a file. The values round-trip through the
// YAML encoder, so a Go string that looks like a date stays a string.
func NewProducerFields(fields map[string]any) (ProducerFields, error) {
	if fields == nil {
		return nil, nil
	}
	data, err := yaml.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("encode producer fields: %w", err)
	}
	var out ProducerFields
	if err := yaml.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("decode producer fields: %w", err)
	}
	return out, nil
}

// ApplyProducerFields merges producer-supplied frontmatter fields into fm,
// implementing the merge policy of the structured create/update contract:
//
//   - nb-owned fields (id, created, modified, type, remote) are silently
//     skipped — nb's values win. Producers routinely render their full
//     frontmatter model into one file; erroring on nb-owned keys would force
//     every producer to special-case them, while accepting them would let a
//     producer forge a note's identity.
//   - Keys that name a known typed field (title, tags, priority, …) are
//     validated for shape and REPLACE the field's value.
//   - Every other key is a namespaced producer field (`pomodoro_*`, `hn_*`,
//     `source`, …) and is carried verbatim into the Extra map, where Build
//     round-trips it losslessly.
func ApplyProducerFields(fm *Frontmatter, fields ProducerFields) error {
	// Deterministic application order so multi-key error reporting is stable.
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		value := fields[key]
		if nbOwnedFields[key] {
			continue // nb wins; see the function comment
		}
		if !knownFieldNames[key] {
			if !producerKeyPattern.MatchString(key) {
				return fmt.Errorf("invalid frontmatter key %q (want a letter followed by letters, digits, '_', '.' or '-')", key)
			}
			if fm.Extra == nil {
				fm.Extra = map[string]yaml.Node{}
			}
			fm.Extra[key] = value
			continue
		}

		var err error
		switch key {
		case "title":
			err = setStringField(&fm.Title, key, value)
		case "aliases":
			err = setStringSliceField(&fm.Aliases, key, value)
		case "tags":
			err = setStringSliceField(&fm.Tags, key, value)
		case "repository":
			err = setStringField(&fm.Repository, key, value)
		case "branch":
			err = setStringField(&fm.Branch, key, value)
		case "worktree":
			err = setStringField(&fm.Worktree, key, value)
		case "started":
			err = setStringField(&fm.Started, key, value)
		case "plan_ref":
			err = setStringField(&fm.PlanRef, key, value)
		case "plan_job":
			err = setStringField(&fm.PlanJob, key, value)
		case "priority":
			err = setStringField(&fm.Priority, key, value)
		case "name":
			err = setStringField(&fm.Name, key, value)
		case "description":
			err = setStringField(&fm.Description, key, value)
		case "publishDate":
			err = setStringField(&fm.PublishDate, key, value)
		case "updatedDate":
			err = setStringField(&fm.UpdatedDate, key, value)
		case "draft":
			err = setBoolField(&fm.Draft, key, value)
		case "featured":
			err = setBoolField(&fm.Featured, key, value)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// setStringField assigns a producer value to a string-typed known field.
//
// The shape check is on the node's resolved TAG, not on whether Decode
// succeeds: yaml.v3 happily decodes an `!!int` scalar into a Go string, so
// `title: 42` would silently become "42". A producer that means the string
// "42" can say so by quoting it — which resolves to !!str and passes.
func setStringField(dst *string, key string, node yaml.Node) error {
	if node.Kind != yaml.ScalarNode || (node.Tag != "" && node.Tag != strTag) {
		return fmt.Errorf("frontmatter field %q must be a string, got %s", key, nodeShape(node))
	}
	*dst = node.Value
	return nil
}

// setBoolField is setStringField's boolean sibling, tag-strict for the same
// reason: `draft: "yes"` is a string, not a request to set the flag.
func setBoolField(dst *bool, key string, node yaml.Node) error {
	if node.Kind != yaml.ScalarNode || node.Tag != boolTag {
		return fmt.Errorf("frontmatter field %q must be a boolean, got %s", key, nodeShape(node))
	}
	var b bool
	if err := node.Decode(&b); err != nil {
		return fmt.Errorf("frontmatter field %q must be a boolean, got %s", key, nodeShape(node))
	}
	*dst = b
	return nil
}

// setStringSliceField decodes a producer list into []string, rejecting both a
// non-list and a list holding a non-string element (element-wise, for the same
// tag reason as setStringField).
func setStringSliceField(dst *[]string, key string, node yaml.Node) error {
	if node.Kind != yaml.SequenceNode {
		return fmt.Errorf("frontmatter field %q must be a list of strings, got %s", key, nodeShape(node))
	}
	out := make([]string, 0, len(node.Content))
	for _, item := range node.Content {
		if item.Kind != yaml.ScalarNode || (item.Tag != "" && item.Tag != strTag) {
			return fmt.Errorf("frontmatter field %q must be a list of strings, got element %s", key, nodeShape(*item))
		}
		out = append(out, item.Value)
	}
	*dst = out
	return nil
}

// The yaml tags this package makes decisions on.
const (
	strTag  = "!!str"
	boolTag = "!!bool"
)

// nodeShape names what a node holds, for producer-facing error messages. The
// yaml tag (`!!int`, `!!map`) is more useful to someone debugging a
// frontmatter file than the Go type the value would have decoded to.
func nodeShape(node yaml.Node) string {
	if node.Tag != "" {
		return node.Tag
	}
	switch node.Kind {
	case yaml.MappingNode:
		return "a mapping"
	case yaml.SequenceNode:
		return "a list"
	default:
		return "an unrecognized value"
	}
}

// ToMap converts the frontmatter to a map suitable for JSON serialization.
// Typed fields are included when non-zero; Extra nodes are decoded to plain Go
// values. The result is read-only and does not share storage with fm.
func (fm *Frontmatter) ToMap() map[string]any {
	m := make(map[string]any)
	if fm.ID != "" {
		m["id"] = fm.ID
	}
	if fm.Title != "" {
		m["title"] = fm.Title
	}
	if fm.Type != "" {
		m["type"] = fm.Type
	}
	if len(fm.Aliases) > 0 {
		m["aliases"] = fm.Aliases
	}
	if len(fm.Tags) > 0 {
		m["tags"] = fm.Tags
	}
	if fm.Repository != "" {
		m["repository"] = fm.Repository
	}
	if fm.Branch != "" {
		m["branch"] = fm.Branch
	}
	if fm.Worktree != "" {
		m["worktree"] = fm.Worktree
	}
	if fm.Created != "" {
		m["created"] = fm.Created
	}
	if fm.Modified != "" {
		m["modified"] = fm.Modified
	}
	if fm.Started != "" {
		m["started"] = fm.Started
	}
	if fm.PlanRef != "" {
		m["plan_ref"] = fm.PlanRef
	}
	if fm.PlanJob != "" {
		m["plan_job"] = fm.PlanJob
	}
	if fm.Priority != "" {
		m["priority"] = fm.Priority
	}
	if fm.Name != "" {
		m["name"] = fm.Name
	}
	if fm.Description != "" {
		m["description"] = fm.Description
	}
	if fm.PublishDate != "" {
		m["publishDate"] = fm.PublishDate
	}
	if fm.UpdatedDate != "" {
		m["updatedDate"] = fm.UpdatedDate
	}
	if fm.Draft {
		m["draft"] = true
	}
	if fm.Featured {
		m["featured"] = true
	}
	if fm.Remote != nil {
		m["remote"] = fm.Remote
	}
	for k, v := range fm.Extra {
		if knownFieldNames[k] {
			continue
		}
		var val any
		if err := v.Decode(&val); err == nil {
			m[k] = val
		}
	}
	return m
}

// MergeTags combines multiple tag sources and removes duplicates
func MergeTags(sources ...[]string) []string {
	seen := make(map[string]bool)
	result := []string{}

	for _, tags := range sources {
		for _, tag := range tags {
			if tag != "" && !seen[tag] {
				seen[tag] = true
				result = append(result, tag)
			}
		}
	}

	return result
}
