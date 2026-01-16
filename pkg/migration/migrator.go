package migration

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/grovetools/nb/pkg/frontmatter"
	"github.com/sirupsen/logrus"
)

type Migrator struct {
	options  MigrationOptions
	analyzer *Analyzer
	report   *MigrationReport
	output   io.Writer
	logger   *logrus.Entry
}

func NewMigrator(options MigrationOptions, basePath string, output io.Writer, logger *logrus.Entry) *Migrator {
	if logger == nil {
		logger = logrus.NewEntry(logrus.New()) // Fallback to a null logger
	}
	return &Migrator{
		options:  options,
		analyzer: NewAnalyzer(basePath),
		report:   NewMigrationReport(),
		output:   output,
		logger:   logger.WithField("sub-component", "migrator"),
	}
}

func (m *Migrator) MigrateFile(filePath string) error {
	m.report.ProcessedFiles++

	issues, err := m.analyzer.AnalyzeNote(filePath)
	if err != nil {
		m.report.AddError(filePath, err)
		return err
	}

	if len(issues) == 0 {
		m.report.SkippedFiles++
		m.logger.WithField("path", filePath).Debug("No migration issues found, skipping.")
		return nil
	}

	if m.options.Verbose {
		fmt.Fprintf(m.output, "\n%s:\n", filePath)
		for _, issue := range issues {
			fmt.Fprintf(m.output, "  - %s: %s\n", issue.Type, issue.Description)
		}
	}

	if m.options.DryRun {
		m.report.IssuesFixed += len(issues)
		return nil
	}

	newPath, err := m.applyFixes(filePath, issues)
	if err != nil {
		m.report.AddError(filePath, err)
		return fmt.Errorf("failed to apply fixes: %w", err)
	}

	m.report.MigratedFiles++
	m.report.IssuesFixed += len(issues)

	if newPath != filePath {
		m.report.CreatedFiles++
		m.report.DeletedFiles++
		if m.options.Verbose {
			fmt.Fprintf(m.output, "  → Renamed to: %s\n", filepath.Base(newPath))
		}
	}

	return nil
}

func (m *Migrator) applyFixes(filePath string, issues []MigrationIssue) (string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return filePath, fmt.Errorf("failed to read file: %w", err)
	}

	fm, bodyContent, _ := frontmatter.Parse(string(content))
	if fm == nil {
		fm = &frontmatter.Frontmatter{
			Aliases: []string{},
			Tags:    []string{},
		}

		if !strings.HasPrefix(bodyContent, "# ") {
			bodyContent = string(content)
		}
	}

	stat, err := os.Stat(filePath)
	if err != nil {
		return filePath, fmt.Errorf("failed to stat file: %w", err)
	}

	for _, issue := range issues {
		switch issue.Type {
		case "invalid_id", "missing_id":
			if expected, ok := issue.Expected.(string); ok {
				fm.ID = expected
			}
		case "missing_title":
			if expected, ok := issue.Expected.(string); ok {
				fm.Title = expected
			}
		case "missing_created":
			fm.Created = frontmatter.FormatTimestamp(stat.ModTime())
		case "missing_modified":
			fm.Modified = frontmatter.FormatTimestamp(stat.ModTime())
		case "missing_tags":
			if expected, ok := issue.Expected.([]string); ok {
				existingTags := make(map[string]bool)
				for _, tag := range fm.Tags {
					existingTags[tag] = true
				}
				for _, tag := range expected {
					if !existingTags[tag] {
						fm.Tags = append(fm.Tags, tag)
					}
				}
			}
		}
	}

	newContent := frontmatter.BuildContent(fm, bodyContent)

	newPath := filePath
	for _, issue := range issues {
		if issue.Type == "non_standard_filename" {
			if expected, ok := issue.Expected.(string); ok {
				newPath = filepath.Join(filepath.Dir(filePath), expected)

				if _, err := os.Stat(newPath); err == nil && newPath != filePath {
					base := strings.TrimSuffix(expected, ".md")
					for i := 2; ; i++ {
						candidate := filepath.Join(filepath.Dir(filePath), fmt.Sprintf("%s-%d.md", base, i))
						if _, err := os.Stat(candidate); os.IsNotExist(err) {
							newPath = candidate
							break
						}
					}
				}
			}
		}
	}

	if !m.options.NoBackup && filePath == newPath {
		backupPath := filePath + ".backup"
		if err := copyFile(filePath, backupPath); err != nil {
			return filePath, fmt.Errorf("failed to create backup: %w", err)
		}
	}

	if err := os.WriteFile(newPath, []byte(newContent), 0644); err != nil {
		return filePath, fmt.Errorf("failed to write file: %w", err)
	}

	if newPath != filePath {
		if err := os.Remove(filePath); err != nil {
			return newPath, fmt.Errorf("failed to remove old file: %w", err)
		}
	}

	if err := os.Chtimes(newPath, stat.ModTime(), stat.ModTime()); err != nil {
		return newPath, fmt.Errorf("failed to preserve timestamps: %w", err)
	}

	return newPath, nil
}

func (m *Migrator) GetReport() *MigrationReport {
	return m.report
}

func (m *Migrator) Complete() {
	m.report.Complete()
}

func copyFile(src, dst string) error {
	input, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, input, 0644)
}
