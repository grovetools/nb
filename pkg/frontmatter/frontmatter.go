package frontmatter

import (
	"fmt"
	"regexp"
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
	PlanRef    string   `yaml:"plan_ref,omitempty"` // Reference to associated plan
	Priority   string   `yaml:"priority,omitempty"` // p0 (most critical) .. p3, empty = none

	// Remote sync metadata
	Remote *RemoteMetadata `yaml:"remote,omitempty"`

	// Blog-specific fields
	Description string `yaml:"description,omitempty"`
	PublishDate string `yaml:"publishDate,omitempty"`
	UpdatedDate string `yaml:"updatedDate,omitempty"`
	Draft       bool   `yaml:"draft,omitempty"`
	Featured    bool   `yaml:"featured,omitempty"`
}

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
	if fm.Priority != "" {
		sb.WriteString(fmt.Sprintf("priority: %s\n", formatYAMLValue(fm.Priority)))
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

	sb.WriteString("---")

	return sb.String()
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
