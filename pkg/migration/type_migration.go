package migration

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// RenameCurrentToInbox scans a notebook for directories named "current"
// and renames them to "inbox", updating the frontmatter of notes within.
func RenameCurrentToInbox(notebookRoot string, options MigrationOptions, output io.Writer) (*MigrationReport, error) {
	report := NewMigrationReport()
	migrator := NewMigrator(options, notebookRoot, output)

	err := filepath.Walk(notebookRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() && info.Name() == "current" {
			inboxPath := filepath.Join(filepath.Dir(path), "inbox")

			if options.DryRun {
				fmt.Fprintf(output, "Would rename directory: %s -> %s\n", path, inboxPath)
				report.MigratedFiles++ // Count directory as a migrated item
			} else {
				if err := os.Rename(path, inboxPath); err != nil {
					report.AddError(path, fmt.Errorf("failed to rename directory: %w", err))
					return nil // Continue walking
				}
				fmt.Fprintf(output, "✓ Renamed directory: %s -> %s\n", path, inboxPath)
			}

			// Now, process files inside the (potentially new) inbox directory
			processDir := inboxPath
			if options.DryRun {
				processDir = path // In dry run, files are still at the old path
			}

			err := filepath.Walk(processDir, func(notePath string, noteInfo os.FileInfo, noteErr error) error {
				if noteErr != nil {
					return noteErr
				}
				if !noteInfo.IsDir() && strings.HasSuffix(noteInfo.Name(), ".md") {
					report.TotalFiles++
					if err := migrator.migrateNoteTypeInFrontmatter(notePath, "current", "inbox"); err != nil {
						report.AddError(notePath, err)
					}
				}
				return nil
			})
			if err != nil {
				report.AddError(processDir, fmt.Errorf("failed walking notes in %s: %w", processDir, err))
			}
			return filepath.SkipDir // Don't descend into the renamed directory again
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	report.Add(migrator.GetReport())
	report.Complete()
	return report, nil
}

// migrateNoteTypeInFrontmatter updates the 'type' and 'tags' fields in a note's frontmatter.
// IMPORTANT: This function preserves ALL frontmatter fields, only updating type and tags.
func (m *Migrator) migrateNoteTypeInFrontmatter(filePath, oldType, newType string) error {
	m.report.ProcessedFiles++

	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	// Use conservative update to preserve all fields
	newContent, modified, err := m.conservativelyUpdateTypeAndTags(string(content), oldType, newType)
	if err != nil {
		m.report.SkippedFiles++
		return nil // Skip files with parse errors
	}

	if !modified {
		m.report.SkippedFiles++
		return nil
	}

	if m.options.DryRun {
		fmt.Fprintf(m.output, "  - Would update frontmatter in: %s\n", filepath.Base(filePath))
		m.report.MigratedFiles++
		m.report.IssuesFixed++
		return nil
	}

	if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
		return err
	}

	m.report.MigratedFiles++
	m.report.IssuesFixed++
	return nil
}

// conservativelyUpdateTypeAndTags updates only type and tags fields while preserving all others
func (m *Migrator) conservativelyUpdateTypeAndTags(content, oldType, newType string) (string, bool, error) {
	// Extract frontmatter using regex
	frontmatterPattern := regexp.MustCompile(`(?s)^---\n(.*?)\n---\n(.*)`)
	matches := frontmatterPattern.FindStringSubmatch(content)

	if len(matches) != 3 {
		// No frontmatter
		return content, false, nil
	}

	frontmatterStr := matches[1]
	bodyContent := matches[2]

	// Parse into a map to preserve ALL fields
	var fmMap map[string]interface{}
	if err := yaml.Unmarshal([]byte(frontmatterStr), &fmMap); err != nil {
		return content, false, err
	}

	modified := false

	// Update type field if it matches oldType
	if typeVal, exists := fmMap["type"]; exists {
		if typeStr, ok := typeVal.(string); ok && typeStr == oldType {
			fmMap["type"] = newType
			modified = true
		}
	}

	// Update tags array
	if tagsVal, exists := fmMap["tags"]; exists {
		if tagsSlice, ok := tagsVal.([]interface{}); ok {
			newTags := []interface{}{}
			hasNewType := false
			tagModified := false

			for _, tag := range tagsSlice {
				if tagStr, ok := tag.(string); ok {
					if tagStr == oldType {
						newTags = append(newTags, newType)
						hasNewType = true
						tagModified = true
					} else {
						if tagStr == newType {
							hasNewType = true
						}
						newTags = append(newTags, tag)
					}
				}
			}

			// Ensure newType is in tags if type field is newType
			if !hasNewType {
				if typeVal, exists := fmMap["type"]; exists {
					if typeStr, ok := typeVal.(string); ok && typeStr == newType {
						newTags = append(newTags, newType)
						tagModified = true
					}
				}
			}

			if tagModified {
				fmMap["tags"] = newTags
				modified = true
			}
		}
	}

	if !modified {
		return content, false, nil
	}

	// Marshal back to YAML, preserving all fields
	updatedFM, err := yaml.Marshal(fmMap)
	if err != nil {
		return content, false, err
	}

	// Rebuild the content
	newContent := "---\n" + string(updatedFM) + "---\n" + bodyContent
	return newContent, true, nil
}

// EnsureTypeInTags scans all notes in a notebook and ensures their note type
// is present in the tags array, matching the behavior of new note creation.
func EnsureTypeInTags(notebookRoot string, options MigrationOptions, output io.Writer) (*MigrationReport, error) {
	report := NewMigrationReport()
	migrator := NewMigrator(options, notebookRoot, output)

	err := filepath.Walk(notebookRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Process markdown files only
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".md") {
			report.TotalFiles++

			// Extract note type from path
			noteType := extractNoteTypeFromPath(path, notebookRoot)
			if noteType == "" {
				// Skip files where we can't determine the type
				report.SkippedFiles++
				return nil
			}

			// Ensure the note type is in tags
			if err := migrator.ensureTypeInTags(path, noteType); err != nil {
				report.AddError(path, err)
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	report.Add(migrator.GetReport())
	report.Complete()
	return report, nil
}

// extractNoteTypeFromPath extracts the note type from a file path.
// For paths like .../repos/workspace/main/inbox/note.md -> "inbox"
// For paths like .../repos/workspace/main/architecture/decisions/note.md -> "architecture/decisions"
func extractNoteTypeFromPath(path, notebookRoot string) string {
	// Normalize paths
	path = filepath.ToSlash(path)
	notebookRoot = filepath.ToSlash(notebookRoot)

	// Get relative path from notebook root
	relPath := strings.TrimPrefix(path, notebookRoot)
	relPath = strings.TrimPrefix(relPath, "/")

	// Split into parts
	parts := strings.Split(relPath, "/")

	// Look for common structural markers
	for i, part := range parts {
		// Skip archive, plans, and chats directories - these are not note types
		if part == ".archive" || part == "archive" || part == "plans" || part == "chats" {
			return ""
		}

		// For repos/workspace/branch/TYPE structure
		if part == "repos" && i+3 < len(parts) {
			// parts[i+1] = workspace name
			// parts[i+2] = branch name
			// parts[i+3+] = note type path
			typeStart := i + 3
			typeEnd := len(parts) - 1 // Exclude filename
			if typeStart < typeEnd {
				return strings.Join(parts[typeStart:typeEnd], "/")
			}
		}

		// For workspaces/workspace/notes/TYPE structure (new notebook structure)
		if part == "workspaces" && i+2 < len(parts) && parts[i+2] == "notes" && i+3 < len(parts) {
			typeStart := i + 3
			typeEnd := len(parts) - 1 // Exclude filename
			if typeStart < typeEnd {
				return strings.Join(parts[typeStart:typeEnd], "/")
			}
		}

		// For global/notes/TYPE structure
		if part == "global" && i+1 < len(parts) {
			if parts[i+1] == "notes" && i+2 < len(parts) {
				typeStart := i + 2
				typeEnd := len(parts) - 1 // Exclude filename
				if typeStart < typeEnd {
					return strings.Join(parts[typeStart:typeEnd], "/")
				}
			} else {
				// Direct type path: global/TYPE/note.md
				typeStart := i + 1
				typeEnd := len(parts) - 1 // Exclude filename
				if typeStart < typeEnd {
					return strings.Join(parts[typeStart:typeEnd], "/")
				}
			}
		}
	}

	return ""
}

// ensureTypeInTags ensures a note's type is present in its tags array.
// IMPORTANT: This function preserves ALL frontmatter fields, only updating tags.
func (m *Migrator) ensureTypeInTags(filePath, noteType string) error {
	m.report.ProcessedFiles++

	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	// Use conservative update to preserve all fields
	newContent, modified, err := m.conservativelyAddTagsForType(string(content), noteType)
	if err != nil {
		m.report.SkippedFiles++
		return nil // Skip files with parse errors
	}

	if !modified {
		m.report.SkippedFiles++
		return nil
	}

	if m.options.DryRun {
		fmt.Fprintf(m.output, "Would add type tags to: %s\n", filepath.Base(filePath))
		m.report.MigratedFiles++
		m.report.IssuesFixed++
		return nil
	}

	if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
		return err
	}

	m.report.MigratedFiles++
	m.report.IssuesFixed++
	return nil
}

// conservativelyAddTagsForType adds missing type tags while preserving all other fields
func (m *Migrator) conservativelyAddTagsForType(content, noteType string) (string, bool, error) {
	// Extract frontmatter using regex
	frontmatterPattern := regexp.MustCompile(`(?s)^---\n(.*?)\n---\n(.*)`)
	matches := frontmatterPattern.FindStringSubmatch(content)

	if len(matches) != 3 {
		// No frontmatter
		return content, false, nil
	}

	frontmatterStr := matches[1]
	bodyContent := matches[2]

	// Parse into a map to preserve ALL fields
	var fmMap map[string]interface{}
	if err := yaml.Unmarshal([]byte(frontmatterStr), &fmMap); err != nil {
		return content, false, err
	}

	// Extract all type components (e.g., "architecture/decisions" -> ["architecture", "decisions"])
	typeParts := strings.Split(noteType, "/")

	// Get existing tags
	existingTags := make(map[string]bool)
	var currentTags []interface{}

	if tagsVal, exists := fmMap["tags"]; exists {
		if tagsSlice, ok := tagsVal.([]interface{}); ok {
			currentTags = tagsSlice
			for _, tag := range tagsSlice {
				if tagStr, ok := tag.(string); ok {
					existingTags[tagStr] = true
				}
			}
		}
	}

	// Find missing parts
	var missingParts []string
	for _, part := range typeParts {
		if !existingTags[part] {
			missingParts = append(missingParts, part)
		}
	}

	// If all parts are present, no modification needed
	if len(missingParts) == 0 {
		return content, false, nil
	}

	// Add missing tags
	for _, part := range missingParts {
		currentTags = append(currentTags, part)
	}
	fmMap["tags"] = currentTags

	// Marshal back to YAML, preserving all fields
	updatedFM, err := yaml.Marshal(fmMap)
	if err != nil {
		return content, false, err
	}

	// Rebuild the content
	newContent := "---\n" + string(updatedFM) + "---\n" + bodyContent
	return newContent, true, nil
}

// Add merges another report's stats into this one.
func (r *MigrationReport) Add(other *MigrationReport) {
	r.TotalFiles += other.TotalFiles
	r.ProcessedFiles += other.ProcessedFiles
	r.MigratedFiles += other.MigratedFiles
	r.SkippedFiles += other.SkippedFiles
	r.FailedFiles += other.FailedFiles
	r.IssuesFixed += other.IssuesFixed
	for path, err := range other.ProcessingErrors {
		r.AddError(path, err)
	}
}
