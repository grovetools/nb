package migration

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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
func (m *Migrator) migrateNoteTypeInFrontmatter(filePath, oldType, newType string) error {
	m.report.ProcessedFiles++

	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	fm, body, err := ParseFrontmatter(string(content))
	if err != nil || fm == nil {
		m.report.SkippedFiles++
		return nil // Skip files without valid frontmatter
	}

	modified := false
	if fm.Type == oldType {
		fm.Type = newType
		modified = true
	}

	// Update tags: replace oldType with newType, and ensure newType is present
	var newTags []string
	tagModified := false
	hasNewType := false

	for _, tag := range fm.Tags {
		if tag == oldType {
			newTags = append(newTags, newType)
			hasNewType = true
			tagModified = true
		} else {
			if tag == newType {
				hasNewType = true
			}
			newTags = append(newTags, tag)
		}
	}

	// Ensure the new type is in tags (consistency with new note creation)
	if !hasNewType && fm.Type == newType {
		newTags = append(newTags, newType)
		tagModified = true
	}

	if tagModified {
		fm.Tags = newTags
		modified = true
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

	newContent := BuildContentWithFrontmatter(fm, body)
	if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
		return err
	}

	m.report.MigratedFiles++
	m.report.IssuesFixed++
	return nil
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
		// Skip archive directories
		if part == ".archive" || part == "archive" {
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
func (m *Migrator) ensureTypeInTags(filePath, noteType string) error {
	m.report.ProcessedFiles++

	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	fm, body, err := ParseFrontmatter(string(content))
	if err != nil || fm == nil {
		m.report.SkippedFiles++
		return nil // Skip files without valid frontmatter
	}

	// Extract all type components (e.g., "architecture/decisions" -> ["architecture", "decisions"])
	typeParts := strings.Split(noteType, "/")

	// Check which parts are missing
	existingTags := make(map[string]bool)
	for _, tag := range fm.Tags {
		existingTags[tag] = true
	}

	missingParts := []string{}
	for _, part := range typeParts {
		if !existingTags[part] {
			missingParts = append(missingParts, part)
		}
	}

	// If all parts are present, no modification needed
	if len(missingParts) == 0 {
		m.report.SkippedFiles++
		return nil
	}

	if m.options.DryRun {
		fmt.Fprintf(m.output, "Would add tags %v to: %s\n", missingParts, filepath.Base(filePath))
		m.report.MigratedFiles++
		m.report.IssuesFixed++
		return nil
	}

	// Add missing type parts to tags
	fm.Tags = append(fm.Tags, missingParts...)

	newContent := BuildContentWithFrontmatter(fm, body)
	if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
		return err
	}

	m.report.MigratedFiles++
	m.report.IssuesFixed++
	return nil
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
