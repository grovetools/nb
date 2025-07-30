package migration

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var (
	datePattern     = regexp.MustCompile(`^\d{8}`)
	markdownHeading = regexp.MustCompile(`^#\s+(.+)`)
	idPattern       = regexp.MustCompile(`^\d{8}-\d{6}`)
)

type Analyzer struct {
	basePath string
}

func NewAnalyzer(basePath string) *Analyzer {
	return &Analyzer{basePath: basePath}
}

func (a *Analyzer) AnalyzeNote(filePath string) ([]MigrationIssue, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	issues := []MigrationIssue{}

	fm, bodyContent, err := ParseFrontmatter(string(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	if fm == nil {
		issues = append(issues, MigrationIssue{
			Type:        "missing_frontmatter",
			Description: "Note is missing frontmatter",
			Field:       "frontmatter",
		})

		fm = &Frontmatter{}
	}

	filename := filepath.Base(filePath)
	filenameStem := strings.TrimSuffix(filename, filepath.Ext(filename))

	if fm.ID == "" || !idPattern.MatchString(fm.ID) {
		issues = append(issues, MigrationIssue{
			Type:        "invalid_id",
			Description: "ID is missing or invalid",
			Field:       "id",
			Current:     fm.ID,
			Expected:    a.generateID(filePath),
		})
	}

	if fm.Title == "" {
		bestTitle := a.extractBestTitle(fm, bodyContent, filenameStem)
		issues = append(issues, MigrationIssue{
			Type:        "missing_title",
			Description: "Title is missing",
			Field:       "title",
			Current:     "",
			Expected:    bestTitle,
		})
	}

	if fm.Created == "" {
		issues = append(issues, MigrationIssue{
			Type:        "missing_created",
			Description: "Created timestamp is missing",
			Field:       "created",
			Current:     "",
			Expected:    FormatTimestamp(time.Now()),
		})
	}

	if fm.Modified == "" {
		issues = append(issues, MigrationIssue{
			Type:        "missing_modified",
			Description: "Modified timestamp is missing",
			Field:       "modified",
			Current:     "",
			Expected:    FormatTimestamp(time.Now()),
		})
	}

	expectedFilename := a.generateStandardFilename(fm, bodyContent, filenameStem)
	if filename != expectedFilename {
		issues = append(issues, MigrationIssue{
			Type:        "non_standard_filename",
			Description: "Filename doesn't follow standard format",
			Field:       "filename",
			Current:     filename,
			Expected:    expectedFilename,
		})
	}

	expectedTags := a.generateTags(filePath)
	if !a.tagsEqual(fm.Tags, expectedTags) && len(expectedTags) > 0 {
		issues = append(issues, MigrationIssue{
			Type:        "missing_tags",
			Description: "Tags don't match directory structure",
			Field:       "tags",
			Current:     fm.Tags,
			Expected:    expectedTags,
		})
	}

	return issues, nil
}

func (a *Analyzer) extractBestTitle(fm *Frontmatter, bodyContent, filenameStem string) string {
	if fm.Title != "" {
		return fm.Title
	}

	lines := strings.Split(bodyContent, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if match := markdownHeading.FindStringSubmatch(line); match != nil {
			return strings.TrimSpace(match[1])
		}
	}

	title := filenameStem
	if datePattern.MatchString(title) && len(title) > 9 {
		title = title[9:]
	}

	title = strings.ReplaceAll(title, "-", " ")
	title = strings.ReplaceAll(title, "_", " ")

	words := strings.Fields(title)
	for i, word := range words {
		if i == 0 || len(word) > 2 {
			words[i] = cases.Title(language.English).String(strings.ToLower(word))
		} else {
			words[i] = strings.ToLower(word)
		}
	}

	return strings.Join(words, " ")
}

func (a *Analyzer) generateStandardFilename(fm *Frontmatter, bodyContent, filenameStem string) string {
	dateStr := ""
	if fm.ID != "" && idPattern.MatchString(fm.ID) {
		parts := strings.Split(fm.ID, "-")
		if len(parts) >= 1 {
			dateStr = parts[0]
		}
	}

	if dateStr == "" && datePattern.MatchString(filenameStem) {
		dateStr = filenameStem[:8]
	}

	if dateStr == "" {
		dateStr = time.Now().Format("20060102")
	}

	title := a.extractBestTitle(fm, bodyContent, filenameStem)
	titleSlug := sanitizeFilename(title)

	return fmt.Sprintf("%s-%s.md", dateStr, titleSlug)
}

func (a *Analyzer) generateID(filePath string) string {
	filename := filepath.Base(filePath)
	filenameStem := strings.TrimSuffix(filename, filepath.Ext(filename))

	if idPattern.MatchString(filenameStem) {
		return filenameStem
	}

	dateStr := time.Now().Format("20060102")
	if datePattern.MatchString(filenameStem) {
		dateStr = filenameStem[:8]
	}

	timeStr := time.Now().Format("150405")
	return fmt.Sprintf("%s-%s", dateStr, timeStr)
}

func (a *Analyzer) generateTags(filePath string) []string {
	relPath, err := filepath.Rel(a.basePath, filePath)
	if err != nil {
		return []string{}
	}

	dir := filepath.Dir(relPath)
	if dir == "." || dir == "/" {
		return []string{}
	}

	parts := strings.Split(dir, string(filepath.Separator))

	tags := []string{}
	for _, part := range parts {
		if part != "" && part != "." && part != ".." {
			tags = append(tags, strings.ToLower(part))
		}
	}

	return tags
}

func (a *Analyzer) tagsEqual(tags1, tags2 []string) bool {
	if len(tags1) != len(tags2) {
		return false
	}

	tagMap := make(map[string]bool)
	for _, tag := range tags1 {
		tagMap[tag] = true
	}

	for _, tag := range tags2 {
		if !tagMap[tag] {
			return false
		}
	}

	return true
}

func sanitizeFilename(s string) string {
	s = strings.ToLower(s)

	replacements := map[string]string{
		" - ": "-",
		"_":   "-",
		" ":   "-",
		"/":   "-",
		"\\":  "-",
		":":   "-",
		"*":   "",
		"?":   "",
		"\"":  "",
		"<":   "",
		">":   "",
		"|":   "",
		"&":   "and",
		"@":   "at",
		"#":   "",
		"%":   "",
		"'":   "",
		"–":   "-",
		"—":   "-",
		"…":   "",
		".":   "",
		",":   "",
		";":   "",
		"!":   "",
		"(":   "",
		")":   "",
		"[":   "",
		"]":   "",
		"{":   "",
		"}":   "",
		"=":   "-",
		"+":   "-",
		"~":   "-",
		"`":   "",
		"$":   "",
		"^":   "",
	}

	for old, new := range replacements {
		s = strings.ReplaceAll(s, old, new)
	}

	re := regexp.MustCompile(`-+`)
	s = re.ReplaceAllString(s, "-")

	s = strings.Trim(s, "-")

	if s == "" {
		s = "untitled"
	}

	const maxLength = 100
	if len(s) > maxLength {
		s = s[:maxLength]
		s = strings.TrimRight(s, "-")
	}

	return s
}
