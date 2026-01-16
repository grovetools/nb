package migration

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"gopkg.in/yaml.v3"
)

// MigrateWorkspaces copies all files from old workspace names to new ones within notebooks,
// updating frontmatter fields like 'repository' and 'tags' for markdown files.
// Non-markdown files (logs, xml, yaml, etc.) are copied as-is.
func MigrateWorkspaces(sourceNotebookRoot, targetNotebookRoot string, renames map[string]string, options MigrationOptions, output io.Writer) (*MigrationReport, error) {
	report := NewMigrationReport()

	for oldName, newName := range renames {
		sourceWorkspacePath := filepath.Join(sourceNotebookRoot, "workspaces", oldName)

		// Check if the source workspace directory exists
		if _, err := os.Stat(sourceWorkspacePath); os.IsNotExist(err) {
			if options.Verbose {
				fmt.Fprintf(output, "Skipping non-existent source workspace: %s\n", sourceWorkspacePath)
			}
			continue
		}

		if options.Verbose {
			fmt.Fprintf(output, "Scanning workspace '%s' to migrate to '%s'...\n", oldName, newName)
		}

		err := filepath.Walk(sourceWorkspacePath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// Skip directories (we'll create them as needed)
			if info.IsDir() {
				return nil
			}

			report.TotalFiles++
			report.ProcessedFiles++

			// Read source file content
			content, err := os.ReadFile(path)
			if err != nil {
				report.AddError(path, fmt.Errorf("failed to read source file: %w", err))
				return nil // Continue walking
			}

			// Determine target path
			relPath, err := filepath.Rel(sourceWorkspacePath, path)
			if err != nil {
				report.AddError(path, fmt.Errorf("failed to get relative path: %w", err))
				return nil // Continue walking
			}
			targetPath := filepath.Join(targetNotebookRoot, "workspaces", newName, relPath)

			// For markdown files, update frontmatter
			var newContent string
			var modified bool
			if filepath.Ext(path) == ".md" {
				newContent, modified, err = conservativelyUpdateRepositoryAndTags(string(content), oldName, newName)
				if err != nil {
					// Log the error but still copy the file as-is
					if options.Verbose {
						fmt.Fprintf(output, "  WARNING: Failed to parse frontmatter in %s: %v (copying as-is)\n", relPath, err)
					}
					newContent = string(content)
					modified = false
				}
			} else {
				// Non-markdown files: copy as-is
				newContent = string(content)
				modified = false
			}

			if options.DryRun {
				ext := filepath.Ext(path)
				if ext == ".md" && modified {
					fmt.Fprintf(output, "  - Would migrate and update: %s -> %s\n", relPath, targetPath)
				} else if ext == ".md" {
					fmt.Fprintf(output, "  - Would copy (no frontmatter changes): %s\n", relPath)
				} else {
					fmt.Fprintf(output, "  - Would copy: %s\n", relPath)
				}
				report.MigratedFiles++
				if modified {
					report.IssuesFixed++
				}
				return nil
			}

			// Create target directory
			targetDir := filepath.Dir(targetPath)
			if err := os.MkdirAll(targetDir, 0755); err != nil {
				report.AddError(path, fmt.Errorf("failed to create target directory %s: %w", targetDir, err))
				return nil // Continue walking
			}

			// Write the file to the target location
			if err := os.WriteFile(targetPath, []byte(newContent), info.Mode()); err != nil {
				report.AddError(path, fmt.Errorf("failed to write target file %s: %w", targetPath, err))
				return nil // Continue walking
			}

			// Preserve timestamps
			if err := os.Chtimes(targetPath, info.ModTime(), info.ModTime()); err != nil {
				if options.Verbose {
					fmt.Fprintf(output, "  WARNING: Failed to preserve timestamps for %s\n", targetPath)
				}
			}

			report.MigratedFiles++
			if modified {
				report.IssuesFixed++
			}
			if options.Verbose {
				ext := filepath.Ext(path)
				if ext == ".md" && modified {
					fmt.Fprintf(output, "  * Migrated: %s\n", relPath)
				} else {
					fmt.Fprintf(output, "  * Copied: %s\n", relPath)
				}
			}

			return nil
		})

		if err != nil {
			report.AddError(sourceWorkspacePath, fmt.Errorf("error walking source workspace: %w", err))
		}
	}

	report.Complete()
	return report, nil
}

// conservativelyUpdateRepositoryAndTags updates the 'repository' field and any matching tags
// in a note's frontmatter, preserving all other fields.
func conservativelyUpdateRepositoryAndTags(content, oldRepo, newRepo string) (string, bool, error) {
	frontmatterPattern := regexp.MustCompile(`(?s)^---\n(.*?)\n---\n(.*)`)
	matches := frontmatterPattern.FindStringSubmatch(content)

	if len(matches) != 3 {
		// No frontmatter, no changes needed
		return content, false, nil
	}

	frontmatterStr := matches[1]
	bodyContent := matches[2]

	var fmMap map[string]interface{}
	if err := yaml.Unmarshal([]byte(frontmatterStr), &fmMap); err != nil {
		return content, false, fmt.Errorf("failed to parse frontmatter yaml: %w", err)
	}

	modified := false

	// Update 'repository' field
	if repoVal, exists := fmMap["repository"]; exists {
		if repoStr, ok := repoVal.(string); ok && repoStr == oldRepo {
			fmMap["repository"] = newRepo
			modified = true
		}
	}

	// Update 'tags' array
	if tagsVal, exists := fmMap["tags"]; exists {
		if tagsSlice, ok := tagsVal.([]interface{}); ok {
			newTags := []interface{}{}
			tagModified := false
			for _, tag := range tagsSlice {
				if tagStr, ok := tag.(string); ok && tagStr == oldRepo {
					newTags = append(newTags, newRepo)
					tagModified = true
				} else {
					newTags = append(newTags, tag)
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

	// Marshal back to YAML to preserve all other fields
	updatedFM, err := yaml.Marshal(fmMap)
	if err != nil {
		return content, false, fmt.Errorf("failed to marshal updated frontmatter: %w", err)
	}

	// Rebuild the full content
	newContent := "---\n" + string(updatedFM) + "---\n" + bodyContent
	return newContent, true, nil
}
