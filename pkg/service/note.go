package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mattsolo1/nb/pkg/frontmatter"
	"github.com/mattsolo1/nb/pkg/models"
)

const (
	// Constants for repeated string literals
	globalWorkspace = "global"
)

// GenerateFilename creates a timestamped filename
func GenerateFilename(suffix string) string {
	// Use YYYYMMDD format for cleaner filenames
	date := time.Now().Format("20060102")
	if suffix != "" {
		return fmt.Sprintf("%s-%s.md", date, sanitizeFilename(suffix))
	}
	return fmt.Sprintf("%s.md", date)
}

// GenerateNoteID creates an ID from filename (without .md extension)
func GenerateNoteID(suffix string) string {
	timestamp := time.Now().Format("20060102-150405")
	if suffix != "" {
		return fmt.Sprintf("%s-%s", timestamp, sanitizeFilename(suffix))
	}
	return timestamp
}

// sanitizeFilename removes invalid characters from filename
func sanitizeFilename(s string) string {
	// Replace spaces with hyphens
	s = strings.ReplaceAll(s, " ", "-")

	// Remove invalid characters
	invalidChars := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	for _, char := range invalidChars {
		s = strings.ReplaceAll(s, char, "")
	}

	// Convert to lowercase and trim
	s = strings.ToLower(strings.TrimSpace(s))

	return s
}

// ParseNote reads and parses a note file
func ParseNote(path string) (*models.Note, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	contentStr := string(content)

	// Parse frontmatter
	fm, _, err := frontmatter.Parse(contentStr)
	if err != nil {
		// If frontmatter parsing fails, continue with default parsing
		fm = nil
	}

	// Extract metadata from path
	workspace, branch, noteType := GetNoteMetadata(path)

	note := &models.Note{
		Path:       path,
		Title:      extractTitle(contentStr),
		Type:       models.NoteType(noteType),
		Workspace:  workspace,
		Branch:     branch,
		CreatedAt:  info.ModTime(), // Could use birthtime if available
		ModifiedAt: info.ModTime(),
		Content:    contentStr,
		WordCount:  countWords(contentStr),
		HasTodos:   containsTodos(contentStr),
		IsArchived: strings.Contains(path, "/archive/"),
	}

	// If frontmatter was successfully parsed, use its data
	if fm != nil {
		if fm.Title != "" {
			note.Title = fm.Title
		}
		if fm.ID != "" {
			note.ID = fm.ID
		}
		if len(fm.Aliases) > 0 {
			note.Aliases = fm.Aliases
		}
		if len(fm.Tags) > 0 {
			note.Tags = fm.Tags
		}
		if fm.Repository != "" {
			note.Repository = fm.Repository
		}
		if fm.Branch != "" {
			note.Branch = fm.Branch
		}

		// Parse timestamps from frontmatter if available
		if fm.Created != "" {
			if t, err := frontmatter.ParseTimestamp(fm.Created); err == nil {
				note.CreatedAt = t
			}
		}
		if fm.Modified != "" {
			if t, err := frontmatter.ParseTimestamp(fm.Modified); err == nil {
				note.ModifiedAt = t
			}
		}
	}

	return note, nil
}

// extractTitle gets the title from markdown content
func extractTitle(content string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimPrefix(line, "# ")
		}
	}
	return "Untitled"
}

// countWords counts words in content
func countWords(content string) int {
	return len(strings.Fields(content))
}

// containsTodos checks if content has todo items
func containsTodos(content string) bool {
	return strings.Contains(content, "- [ ]") || strings.Contains(content, "- [x]")
}

// NoteContentGenerator defines the function signature for note content generators
type NoteContentGenerator func(title, workspace, branch string, pathTags []string, now time.Time, timestampStr string) string

// noteContentGenerators maps note types to their content generator functions
var noteContentGenerators = map[models.NoteType]NoteContentGenerator{
	models.NoteTypeQuick:   generateQuickContent,
	models.NoteTypeLLM:     generateLLMContent,
	models.NoteTypeDaily:   generateDailyContent,
	models.NoteTypeLearn:   generateLearnContent,
	models.NoteTypeBlog:    generateBlogContent,
	models.NoteTypePrompts: generatePromptsContent,
}

// generateQuickContent creates content for quick notes
func generateQuickContent(title, workspace, branch string, pathTags []string, now time.Time, timestampStr string) string {
	// For quick notes, use the title directly as ID (which already contains the timestamp + quick)
	fm := &frontmatter.Frontmatter{
		ID:       title,
		Title:    title,
		Aliases:  []string{},
		Tags:     pathTags,
		Created:  timestampStr,
		Modified: timestampStr,
	}

	// Add repository and branch if not in global workspace
	if workspace != "" && workspace != globalWorkspace {
		fm.Repository = workspace
		if branch != "" {
			fm.Branch = branch
		}
	}

	body := fmt.Sprintf("# %s\n\n", title)
	return frontmatter.BuildContent(fm, body)
}

// generateLLMContent creates content for LLM chat notes
func generateLLMContent(title, workspace, branch string, pathTags []string, now time.Time, timestampStr string) string {
	id := fmt.Sprintf("%s-chat%d", now.Format("20060102-150405"), now.Unix()%1000)
	fm := &frontmatter.Frontmatter{
		ID:         id,
		Title:      title,
		Aliases:    []string{},
		Tags:       pathTags,
		Repository: workspace,
		Branch:     branch,
		Created:    timestampStr,
		Modified:   timestampStr,
		Started:    timestampStr,
	}

	body := fmt.Sprintf("# %s\n\n", title)
	return frontmatter.BuildContent(fm, body)
}

// generateDailyContent creates content for daily notes
func generateDailyContent(title, workspace, branch string, pathTags []string, now time.Time, timestampStr string) string {
	id := GenerateNoteID(fmt.Sprintf("daily-%s", now.Format("2006-01-02")))
	fm := &frontmatter.Frontmatter{
		ID:       id,
		Title:    title,
		Aliases:  []string{},
		Tags:     pathTags,
		Created:  timestampStr,
		Modified: timestampStr,
	}

	// Add repository and branch if not in global workspace
	if workspace != "" && workspace != globalWorkspace {
		fm.Repository = workspace
		if branch != "" {
			fm.Branch = branch
		}
	}

	body := fmt.Sprintf(`# Daily Note: %s

## Tasks
- [ ] 

## Notes

## Tomorrow

`, now.Format("2006-01-02"))
	return frontmatter.BuildContent(fm, body)
}

// generateLearnContent creates content for learning notes
func generateLearnContent(title, workspace, branch string, pathTags []string, now time.Time, timestampStr string) string {
	id := GenerateNoteID(title)
	fm := &frontmatter.Frontmatter{
		ID:       id,
		Title:    title,
		Aliases:  []string{},
		Tags:     pathTags,
		Created:  timestampStr,
		Modified: timestampStr,
	}

	// Add repository and branch if not in global workspace
	if workspace != "" && workspace != globalWorkspace {
		fm.Repository = workspace
		if branch != "" {
			fm.Branch = branch
		}
	}

	body := fmt.Sprintf(`# %s

## Summary

## Key Concepts

## Examples

## References

`, title)
	return frontmatter.BuildContent(fm, body)
}

// generateBlogContent creates content for blog posts
func generateBlogContent(title, workspace, branch string, pathTags []string, now time.Time, timestampStr string) string {
	id := GenerateNoteID(title)
	// Match the schema for blog posts
	fm := &frontmatter.Frontmatter{
		ID:          id,
		Title:       title,
		Description: "", // User will fill this in
		PublishDate: frontmatter.FormatTimestamp(now),
		UpdatedDate: frontmatter.FormatTimestamp(now),
		Tags:        pathTags,
		Draft:       true, // Posts are drafts by default
		Featured:    false,
		Aliases:     []string{},
		Created:     timestampStr,
		Modified:    timestampStr,
	}

	// Blog posts are global notes, so no repository/branch
	// Add repository and branch if not in global workspace
	if workspace != "" && workspace != globalWorkspace {
		fm.Repository = workspace
		if branch != "" {
			fm.Branch = branch
		}
	}

	body := fmt.Sprintf(`# %s

Start writing your amazing blog post here.

`, title)
	return frontmatter.BuildContent(fm, body)
}

// generatePromptsContent creates content for reusable LLM prompts
func generatePromptsContent(title, workspace, branch string, pathTags []string, now time.Time, timestampStr string) string {
	id := GenerateNoteID(title)
	fm := &frontmatter.Frontmatter{
		ID:       id,
		Title:    title,
		Aliases:  []string{},
		Tags:     pathTags,
		Created:  timestampStr,
		Modified: timestampStr,
	}

	// Add repository and branch if not in global workspace
	if workspace != "" && workspace != globalWorkspace {
		fm.Repository = workspace
		if branch != "" {
			fm.Branch = branch
		}
	}

	body := fmt.Sprintf(`# %s

## Purpose
<!-- Describe what this prompt is designed to achieve -->

## Prompt

<!-- Your reusable prompt goes here -->

## Usage Notes
<!-- How to use this prompt effectively, any parameters to customize, etc. -->

## Examples
<!-- Example inputs/outputs or use cases -->

`, title)
	return frontmatter.BuildContent(fm, body)
}

// generateDefaultContent creates content for all other note types
func generateDefaultContent(title, workspace, branch string, pathTags []string, now time.Time, timestampStr string) string {
	id := GenerateNoteID(title)
	fm := &frontmatter.Frontmatter{
		ID:       id,
		Title:    title,
		Aliases:  []string{},
		Tags:     pathTags,
		Created:  timestampStr,
		Modified: timestampStr,
	}

	// Add repository and branch if not in global workspace
	if workspace != "" && workspace != globalWorkspace {
		fm.Repository = workspace
		if branch != "" {
			fm.Branch = branch
		}
	}

	body := fmt.Sprintf("# %s\n\n", title)
	return frontmatter.BuildContent(fm, body)
}

// CreateNoteContent generates initial note content based on type and template
func CreateNoteContent(noteType models.NoteType, title, workspace, branch string, template string) string {
	// Extract path components for tags
	pathTags := frontmatter.ExtractPathTags(string(noteType))
	if template != "" {
		// Simple template variable replacement
		replacements := map[string]string{
			"{{.Title}}":     title,
			"{{.Timestamp}}": time.Now().Format("2006-01-02 15:04:05"),
			"{{.Date}}":      time.Now().Format("2006-01-02"),
			"{{.Workspace}}": workspace,
			"{{.Branch}}":    branch,
		}

		result := template
		for key, value := range replacements {
			result = strings.ReplaceAll(result, key, value)
		}
		return result
	}

	// Default templates
	now := time.Now()
	timestampStr := frontmatter.FormatTimestamp(now)

	// Look up the generator function
	if generator, ok := noteContentGenerators[noteType]; ok {
		return generator(title, workspace, branch, pathTags, now, timestampStr)
	}

	// Fallback to default generator if the type is not in the map
	return generateDefaultContent(title, workspace, branch, pathTags, now, timestampStr)
}

// GetNoteMetadata extracts metadata from note path
func GetNoteMetadata(path string) (workspace, branch, noteType string) {
	parts := strings.Split(filepath.ToSlash(path), "/")

	for i, part := range parts {
		if part == "nb" && i+1 < len(parts) {
			if parts[i+1] == globalWorkspace && i+2 < len(parts) {
				// For global notes, everything after "global/" until the filename is the note type
				typeStartIdx := i + 2
				// Find where the filename starts (contains .md)
				for j := len(parts) - 1; j >= typeStartIdx; j-- {
					if strings.HasSuffix(parts[j], ".md") {
						// Join all parts between global and the filename
						noteType = strings.Join(parts[typeStartIdx:j], "/")
						return "global", "", noteType
					}
				}
			}
			if parts[i+1] == "repos" && i+4 < len(parts) {
				workspace = parts[i+2]
				branch = parts[i+3]
				// Everything after the branch until the filename is the note type
				typeStartIdx := i + 4
				// Find where the filename starts (contains .md)
				for j := len(parts) - 1; j >= typeStartIdx; j-- {
					if strings.HasSuffix(parts[j], ".md") {
						// Join all parts between branch and the filename
						noteType = strings.Join(parts[typeStartIdx:j], "/")
						return workspace, branch, noteType
					}
				}
			}
		}
	}

	return "", "", ""
}
