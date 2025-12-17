package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mattsolo1/grove-tend/pkg/assert"
	"github.com/mattsolo1/grove-tend/pkg/fs"
	"github.com/mattsolo1/grove-tend/pkg/git"
	"github.com/mattsolo1/grove-tend/pkg/harness"
)

// NotebookFileBrowserScenario verifies the file-browser-like behavior of `nb`.
func NotebookFileBrowserScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "notebook-file-browser-mode",
		Description: "Verifies dynamic note types and listing of all file types.",
		Tags:        []string{"notebook", "file-browser", "dynamic-types"},
		Steps: []harness.Step{
			{
				Name: "Setup environment with dynamic note type directory",
				Func: func(ctx *harness.Context) error {
					// Setup centralized notebook config without any defined note types
					globalYAML := `
version: "1.0"
notebooks:
  rules:
    default: "main"
  definitions:
    main:
      root_dir: "~/.grove/notebooks/nb"
`
					globalConfigDir := filepath.Join(ctx.HomeDir(), ".config", "grove")
					if err := fs.CreateDir(globalConfigDir); err != nil {
						return err
					}
					if err := fs.WriteString(filepath.Join(globalConfigDir, "grove.yml"), globalYAML); err != nil {
						return err
					}

					// Setup test project
					projectDir := ctx.NewDir("fb-project")
					if err := fs.WriteString(filepath.Join(projectDir, "grove.yml"), "name: fb-project\nversion: '1.0'"); err != nil {
						return err
					}
					if _, err := git.SetupTestRepo(projectDir); err != nil {
						return err
					}
					ctx.Set("project_dir", projectDir)

					// Manually create note type directories and a generic file
					workspaceRoot := filepath.Join(ctx.HomeDir(), ".grove", "notebooks", "nb", "workspaces", "fb-project")
					if err := fs.CreateDir(filepath.Join(workspaceRoot, "inbox")); err != nil {
						return err
					}
					if err := fs.CreateDir(filepath.Join(workspaceRoot, "meetings")); err != nil {
						return err
					}
					if err := fs.WriteString(filepath.Join(workspaceRoot, "inbox", "plain.txt"), "hello world"); err != nil {
						return err
					}

					return nil
				},
			},
			{
				Name: "Verify note creation in dynamic type",
				Func: func(ctx *harness.Context) error {
					nbBin, err := findProjectBinary()
					if err != nil {
						return err
					}
					projectDir := ctx.GetString("project_dir")

					cmd := ctx.Command(nbBin, "new", "--type", "meetings", "--no-edit", "Daily Standup").Dir(projectDir)
					result := cmd.Run()
					ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
					if result.Error != nil {
						return result.Error
					}

					// Verify file was created in the correct directory
					meetingsDir := filepath.Join(ctx.HomeDir(), ".grove", "notebooks", "nb", "workspaces", "fb-project", "meetings")
					files, err := fs.ListFiles(meetingsDir)
					if err != nil {
						return err
					}
					if err := assert.Equal(1, len(files), "expected one note file in meetings dir"); err != nil {
						return err
					}

					notePath := filepath.Join(meetingsDir, files[0])
					content, err := fs.ReadString(notePath)
					if err != nil {
						return err
					}

					// Verify the note has proper frontmatter
					if err := assert.Contains(content, "title: Daily Standup", "frontmatter should contain the title"); err != nil {
						return err
					}
					return assert.Contains(content, "# Daily Standup", "body should contain the h1 heading")
				},
			},
			{
				Name: "Verify listing of all file types",
				Func: func(ctx *harness.Context) error {
					nbBin, err := findProjectBinary()
					if err != nil {
						return err
					}
					projectDir := ctx.GetString("project_dir")

					cmd := ctx.Command(nbBin, "list", "--all", "--json").Dir(projectDir)
					result := cmd.Run()
					ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
					if result.Error != nil {
						return result.Error
					}

					var notes []map[string]interface{}
					if err := json.Unmarshal([]byte(result.Stdout), &notes); err != nil {
						return fmt.Errorf("failed to parse json output: %w", err)
					}

					if err := assert.Equal(2, len(notes), "expected two files to be listed"); err != nil {
						return err
					}

					var mdNote, txtNote map[string]interface{}
					for _, note := range notes {
						if strings.HasSuffix(note["path"].(string), ".md") {
							mdNote = note
						} else if strings.HasSuffix(note["path"].(string), ".txt") {
							txtNote = note
						}
					}

					if mdNote == nil {
						return fmt.Errorf("markdown note not found in list output")
					}
					if txtNote == nil {
						return fmt.Errorf("text file note not found in list output")
					}

					// Assert markdown note properties
					match, _ := regexp.MatchString(`\d{8}-daily-standup.md`, mdNote["title"].(string))
					if err := assert.True(match, "markdown note title should be the filename"); err != nil {
						return err
					}
					if err := assert.Equal("Daily Standup", mdNote["frontmatter_title"].(string), "markdown note frontmatter_title is incorrect"); err != nil {
						return err
					}

					// Assert text file properties
					if err := assert.Equal("plain.txt", txtNote["title"].(string), "text note title should be filename"); err != nil {
						return err
					}
					return assert.Equal("txt", txtNote["type"].(string), "text note type should be file extension")
				},
			},
			{
				Name: "Verify shell completion for dynamic types",
				Func: func(ctx *harness.Context) error {
					nbBin, err := findProjectBinary()
					if err != nil {
						return err
					}
					projectDir := ctx.GetString("project_dir")

					// Using cobra's __complete command
					cmd := ctx.Command(nbBin, "__complete", "new", "--type", "").Dir(projectDir)
					result := cmd.Run()
					ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
					if result.Error != nil {
						return result.Error
					}

					// Output contains completions, one per line
					// The format is just the completion values separated by newlines
					return assert.Contains(result.Stdout, "meetings", "shell completion should offer 'meetings'")
				},
			},
		},
	}
}
