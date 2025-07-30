package migration

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func Migrate(basePath string, options MigrationOptions, output io.Writer) (*MigrationReport, error) {
	if output == nil {
		output = os.Stdout
	}

	migrator := NewMigrator(options, basePath, output)

	var paths []string

	switch {
	case options.Scope.Context != "":
		contextPath := filepath.Join(basePath, options.Scope.Context)
		if err := filepath.Walk(contextPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && strings.HasSuffix(path, ".md") {
				paths = append(paths, path)
			}
			return nil
		}); err != nil {
			return migrator.GetReport(), fmt.Errorf("failed to walk context directory: %w", err)
		}

	case options.Scope.Workspace != "":
		workspacePath := filepath.Join(basePath, "repos", options.Scope.Workspace)
		if err := filepath.Walk(workspacePath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && strings.HasSuffix(path, ".md") {
				paths = append(paths, path)
			}
			return nil
		}); err != nil {
			return migrator.GetReport(), fmt.Errorf("failed to walk workspace directory: %w", err)
		}

	case options.Scope.Global:
		globalPath := filepath.Join(basePath, "global")
		if err := filepath.Walk(globalPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && strings.HasSuffix(path, ".md") {
				paths = append(paths, path)
			}
			return nil
		}); err != nil {
			return migrator.GetReport(), fmt.Errorf("failed to walk global directory: %w", err)
		}

	case options.Scope.All:
		if err := filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && strings.HasSuffix(path, ".md") {
				paths = append(paths, path)
			}
			return nil
		}); err != nil {
			return migrator.GetReport(), fmt.Errorf("failed to walk all directories: %w", err)
		}

	default:
		return migrator.GetReport(), fmt.Errorf("no scope specified")
	}

	migrator.report.TotalFiles = len(paths)

	for _, path := range paths {
		if err := migrator.MigrateFile(path); err != nil {
			if options.Verbose {
				fmt.Fprintf(output, "✗ Error processing %s: %v\n", path, err)
			}
		}
	}

	migrator.Complete()

	return migrator.GetReport(), nil
}

func MigrateFile(filePath, basePath string, options MigrationOptions, output io.Writer) error {
	if output == nil {
		output = os.Stdout
	}

	migrator := NewMigrator(options, basePath, output)
	migrator.report.TotalFiles = 1

	err := migrator.MigrateFile(filePath)
	migrator.Complete()

	return err
}

func AnalyzeFile(filePath, basePath string) ([]MigrationIssue, error) {
	analyzer := NewAnalyzer(basePath)
	return analyzer.AnalyzeNote(filePath)
}
