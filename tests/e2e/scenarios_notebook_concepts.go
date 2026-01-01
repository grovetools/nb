package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mattsolo1/grove-tend/pkg/fs"
	"github.com/mattsolo1/grove-tend/pkg/git"
	"github.com/mattsolo1/grove-tend/pkg/harness"
	"github.com/mattsolo1/grove-tend/pkg/verify"
)

// NotebookConceptBasicScenario verifies basic concept creation and structure.
func NotebookConceptBasicScenario() *harness.Scenario {
	return harness.NewScenario(
		"notebook-concept-basic",
		"Verifies basic concept creation with manifest and overview files.",
		[]string{"notebook", "concepts", "create"},
		[]harness.Step{
			harness.NewStep("Create concept and verify structure", func(ctx *harness.Context) error {
				// 1. Setup global config for centralized notebook
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
					return fmt.Errorf("failed to create global config dir: %w", err)
				}
				if err := fs.WriteString(filepath.Join(globalConfigDir, "grove.yml"), globalYAML); err != nil {
					return err
				}

				// 2. Setup test project
				projectDir := ctx.NewDir("test-project")
				if err := fs.WriteString(filepath.Join(projectDir, "grove.yml"), "name: test-project\nversion: '1.0'"); err != nil {
					return err
				}
				if _, err := git.SetupTestRepo(projectDir); err != nil {
					return err
				}

				// 3. Create a concept using nb concept new
				cmd := ctx.Bin("concept", "new", "My Test Concept").Dir(projectDir)
				result := cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				if result.Error != nil {
					return result.Error
				}

				// 4. Verify output message
				if !strings.Contains(result.Stdout, "Created concept:") {
					return fmt.Errorf("expected 'Created concept:' in output, got: %s", result.Stdout)
				}

				// 5. Verify concept directory was created in the centralized notebook
				conceptDir := filepath.Join(ctx.HomeDir(), ".grove", "notebooks", "nb", "workspaces", "test-project", "concepts", "my-test-concept")
				if err := ctx.Check("concept directory exists", fs.AssertExists(conceptDir)); err != nil {
					return err
				}

				// Verify manifest file
				manifestPath := filepath.Join(conceptDir, "concept-manifest.yml")
				if err := ctx.Check("concept-manifest.yml exists", fs.AssertExists(manifestPath)); err != nil {
					return err
				}

				manifestContent, err := fs.ReadString(manifestPath)
				if err != nil {
					return err
				}

				// Verify overview file
				overviewPath := filepath.Join(conceptDir, "overview.md")
				if err := ctx.Check("overview.md exists", fs.AssertExists(overviewPath)); err != nil {
					return err
				}

				overviewContent, err := fs.ReadString(overviewPath)
				if err != nil {
					return err
				}

				return ctx.Verify(func(v *verify.Collector) {
					v.Contains("manifest has id field", manifestContent, "id: my-test-concept")
					v.Contains("manifest has title field", manifestContent, "title: My Test Concept")
					v.Contains("manifest has related_concepts field", manifestContent, "related_concepts: []")
					v.Contains("manifest has related_plans field", manifestContent, "related_plans: []")
					v.Contains("manifest has related_notes field", manifestContent, "related_notes: []")
					v.Contains("overview has title", overviewContent, "# Overview: My Test Concept")
					v.Contains("overview has summary section", overviewContent, "## Summary")
				})
			}),
		},
	)
}

// NotebookConceptListScenario verifies listing concepts.
func NotebookConceptListScenario() *harness.Scenario {
	return harness.NewScenario(
		"notebook-concept-list",
		"Verifies listing concepts in a workspace.",
		[]string{"notebook", "concepts", "list"},
		[]harness.Step{
			harness.NewStep("Create multiple concepts and list them", func(ctx *harness.Context) error {
				// 1. Setup global config for centralized notebook
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
					return fmt.Errorf("failed to create global config dir: %w", err)
				}
				if err := fs.WriteString(filepath.Join(globalConfigDir, "grove.yml"), globalYAML); err != nil {
					return err
				}

				// 2. Setup test project
				projectDir := ctx.NewDir("test-project")
				if err := fs.WriteString(filepath.Join(projectDir, "grove.yml"), "name: test-project\nversion: '1.0'"); err != nil {
					return err
				}
				if _, err := git.SetupTestRepo(projectDir); err != nil {
					return err
				}

				// Create first concept
				cmd := ctx.Bin("concept", "new", "Authentication").Dir(projectDir)
				result := cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				if result.Error != nil {
					return result.Error
				}

				// Create second concept
				cmd = ctx.Bin("concept", "new", "Database Schema").Dir(projectDir)
				result = cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				if result.Error != nil {
					return result.Error
				}

				// List concepts
				cmd = ctx.Bin("concept", "list").Dir(projectDir)
				result = cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				if result.Error != nil {
					return result.Error
				}

				return ctx.Verify(func(v *verify.Collector) {
					v.Contains("lists first concept", result.Stdout, "authentication")
					v.Contains("lists second concept", result.Stdout, "database-schema")
					v.Contains("shows concept count", result.Stdout, "Concepts (2)")
				})
			}),
		},
	)
}

// NotebookConceptLinkingScenario verifies linking concepts, plans, and notes.
func NotebookConceptLinkingScenario() *harness.Scenario {
	return harness.NewScenario(
		"notebook-concept-linking",
		"Verifies linking concepts to plans, notes, and other concepts.",
		[]string{"notebook", "concepts", "link"},
		[]harness.Step{
			harness.NewStep("Create concepts and link them together", func(ctx *harness.Context) error {
				// 1. Setup global config for centralized notebook
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
					return fmt.Errorf("failed to create global config dir: %w", err)
				}
				if err := fs.WriteString(filepath.Join(globalConfigDir, "grove.yml"), globalYAML); err != nil {
					return err
				}

				// 2. Setup test project
				projectDir := ctx.NewDir("test-project")
				if err := fs.WriteString(filepath.Join(projectDir, "grove.yml"), "name: test-project\nversion: '1.0'"); err != nil {
					return err
				}
				if _, err := git.SetupTestRepo(projectDir); err != nil {
					return err
				}

				// Create source concept
				cmd := ctx.Bin("concept", "new", "Authentication").Dir(projectDir)
				result := cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				if result.Error != nil {
					return result.Error
				}

				// Create target concept
				cmd = ctx.Bin("concept", "new", "Authorization").Dir(projectDir)
				result = cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				if result.Error != nil {
					return result.Error
				}

				// Link concepts together
				cmd = ctx.Bin("concept", "link", "concept", "authentication", "authorization").Dir(projectDir)
				result = cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				if result.Error != nil {
					return result.Error
				}

				// Verify link in manifest
				manifestPath := filepath.Join(ctx.HomeDir(), ".grove", "notebooks", "nb", "workspaces", "test-project", "concepts", "authentication", "concept-manifest.yml")
				manifestContent, err := fs.ReadString(manifestPath)
				if err != nil {
					return err
				}

				return ctx.Verify(func(v *verify.Collector) {
					v.Contains("command shows success", result.Stdout, "Linked concept 'authorization' to concept 'authentication'")
					v.Contains("manifest contains linked concept", manifestContent, "authorization")
				})
			}),
		},
	)
}
