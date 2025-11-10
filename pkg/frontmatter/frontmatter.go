package frontmatter

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var frontmatterPattern = regexp.MustCompile(`(?s)^---\n(.*?)\n---\n(.*)`)

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
	Started    string   `yaml:"started,omitempty"` // For LLM notes

	// Blog-specific fields
	Description string `yaml:"description,omitempty"`
	PublishDate string   `yaml:"publishDate,omitempty"`
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
	sb.WriteString(fmt.Sprintf("id: %s\n", fm.ID))
	sb.WriteString(fmt.Sprintf("title: %s\n", fm.Title))
	if fm.Type != "" {
		sb.WriteString(fmt.Sprintf("type: %s\n", fm.Type))
	}
	sb.WriteString(fmt.Sprintf("aliases: %s\n", formatYAMLArray(fm.Aliases)))
	sb.WriteString(fmt.Sprintf("tags: %s\n", formatYAMLArray(fm.Tags)))

	// Optional fields
	if fm.Repository != "" {
		sb.WriteString(fmt.Sprintf("repository: %s\n", fm.Repository))
	}
	if fm.Branch != "" {
		sb.WriteString(fmt.Sprintf("branch: %s\n", fm.Branch))
	}
	if fm.Worktree != "" {
		sb.WriteString(fmt.Sprintf("worktree: %s\n", fm.Worktree))
	}

	// Timestamps
	sb.WriteString(fmt.Sprintf("created: %s\n", fm.Created))
	sb.WriteString(fmt.Sprintf("modified: %s\n", fm.Modified))

	// Special fields
	if fm.Started != "" {
		sb.WriteString(fmt.Sprintf("started: %s\n", fm.Started))
	}

	// Blog-specific fields
	if fm.Description != "" {
		sb.WriteString(fmt.Sprintf("description: %s\n", fm.Description))
	}
	if fm.PublishDate != "" {
		sb.WriteString(fmt.Sprintf("publishDate: %s\n", fm.PublishDate))
	}
	if fm.UpdatedDate != "" {
		sb.WriteString(fmt.Sprintf("updatedDate: %s\n", fm.UpdatedDate))
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

// FormatTimestamp formats a time.Time into the standard frontmatter timestamp format
func FormatTimestamp(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}

// ParseTimestamp parses a frontmatter timestamp string into time.Time
func ParseTimestamp(s string) (time.Time, error) {
	return time.Parse("2006-01-02 15:04:05", s)
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

// needsQuoting checks if a string needs to be quoted in YAML
func needsQuoting(s string) bool {
	return strings.ContainsAny(s, ",:[]{}\"'")
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
