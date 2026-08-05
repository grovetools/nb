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
	// `pomodoro_block_id` or flow's `status`, so external producers could
	// never trust nb with structured metadata. Build re-emits these keys in
	// sorted order after the known fields, so the round-trip is lossless AND
	// deterministic.
	Extra map[string]any `yaml:",inline"`
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
	"priority": true, "name": true, "remote": true, "description": true,
	"publishDate": true, "updatedDate": true, "draft": true, "featured": true,
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

// marshalExtraEntry renders one extension key as YAML. Values go through the
// real YAML encoder (2-space indent, matching the rest of the codebase) rather
// than the hand-rolled formatYAMLValue, because producer values can be
// numbers, booleans, lists, or nested maps — not just strings.
func marshalExtraEntry(key string, value any) string {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(map[string]any{key: value}); err != nil {
		// An unencodable value (e.g. a channel smuggled in programmatically)
		// is dropped rather than corrupting the note file. File-loaded
		// producer values can never hit this: anything YAML/JSON parsed is
		// YAML-encodable.
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

// LoadProducerFields reads a producer frontmatter file (--frontmatter-file)
// and returns its top-level fields. The file may be JSON or YAML — JSON is a
// subset of YAML, so one unmarshal handles both — but its root must be a
// mapping; anything else is a malformed producer file, not a note.
func LoadProducerFields(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read frontmatter file: %w", err)
	}
	var fields map[string]any
	if err := yaml.Unmarshal(data, &fields); err != nil {
		return nil, fmt.Errorf("parse frontmatter file %s (JSON or YAML mapping expected): %w", path, err)
	}
	return fields, nil
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
func ApplyProducerFields(fm *Frontmatter, fields map[string]any) error {
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
				fm.Extra = map[string]any{}
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

// setStringField assigns a producer value to a string-typed known field,
// rejecting non-string shapes with the offending key named.
func setStringField(dst *string, key string, value any) error {
	s, ok := value.(string)
	if !ok {
		return fmt.Errorf("frontmatter field %q must be a string, got %T", key, value)
	}
	*dst = s
	return nil
}

// setBoolField is setStringField's boolean sibling.
func setBoolField(dst *bool, key string, value any) error {
	b, ok := value.(bool)
	if !ok {
		return fmt.Errorf("frontmatter field %q must be a boolean, got %T", key, value)
	}
	*dst = b
	return nil
}

// setStringSliceField coerces a producer list into []string. YAML/JSON
// unmarshal yields []any, so each element is checked individually.
func setStringSliceField(dst *[]string, key string, value any) error {
	switch v := value.(type) {
	case []string:
		*dst = v
		return nil
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return fmt.Errorf("frontmatter field %q must be a list of strings, got element of type %T", key, item)
			}
			out = append(out, s)
		}
		*dst = out
		return nil
	default:
		return fmt.Errorf("frontmatter field %q must be a list of strings, got %T", key, value)
	}
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
