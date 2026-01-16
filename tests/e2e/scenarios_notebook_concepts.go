package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/grovetools/tend/pkg/fs"
	"github.com/grovetools/tend/pkg/git"
	"github.com/grovetools/tend/pkg/harness"
	"github.com/grovetools/tend/pkg/verify"
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

					// Verify frontmatter in overview.md
					v.Contains("overview starts with frontmatter delimiter", overviewContent, "---")
					v.Contains("overview frontmatter has title", overviewContent, "title: My Test Concept")
					v.Contains("overview frontmatter has type", overviewContent, "type: concepts")
					v.Contains("overview frontmatter has tags", overviewContent, "tags: [concepts, test-project]")
					v.Contains("overview frontmatter has repository", overviewContent, "repository: test-project")

					// Verify body content
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

// NotebookConceptAliasLinkingScenario verifies linking concepts using aliases.
func NotebookConceptAliasLinkingScenario() *harness.Scenario {
	return harness.NewScenario(
		"notebook-concept-alias-linking",
		"Verifies linking concepts to plans and notes using workspace aliases.",
		[]string{"notebook", "concepts", "alias", "link"},
		[]harness.Step{
			harness.NewStep("Create concept and link to plan and note using aliases", func(ctx *harness.Context) error {
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

				// 3. Create a plan directory structure
				plansDir := filepath.Join(projectDir, "plans", "my-test-plan")
				if err := fs.CreateDir(plansDir); err != nil {
					return err
				}
				planContent := "# Test Plan\n\nThis is a test plan for alias linking."
				if err := fs.WriteString(filepath.Join(plansDir, "01-spec.md"), planContent); err != nil {
					return err
				}

				// 4. Create a note in the centralized notebook
				cmd := ctx.Bin("new", "--no-edit", "test note for linking").Dir(projectDir)
				result := cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				if result.Error != nil {
					return result.Error
				}

				// Find the created note path from the centralized notebook
				notesDir := filepath.Join(ctx.HomeDir(), ".grove", "notebooks", "nb", "workspaces", "test-project", "inbox")
				noteFiles, err := fs.ListFiles(notesDir)
				if err != nil {
					return fmt.Errorf("failed to list note files: %w", err)
				}
				if len(noteFiles) != 1 {
					return fmt.Errorf("expected 1 note file, got %d", len(noteFiles))
				}
				notePath := noteFiles[0]
				noteBasename := filepath.Base(notePath)

				// 5. Create a concept
				cmd = ctx.Bin("concept", "new", "Architecture Overview").Dir(projectDir)
				result = cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				if result.Error != nil {
					return result.Error
				}

				// 6. Link the plan using an alias
				cmd = ctx.Bin("concept", "link", "plan", "architecture-overview", "test-project:plans/my-test-plan").Dir(projectDir)
				result = cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				if result.Error != nil {
					return result.Error
				}

				// 7. Link the note using an alias
				noteAlias := fmt.Sprintf("test-project:inbox/%s", noteBasename)
				cmd = ctx.Bin("concept", "link", "note", "architecture-overview", noteAlias).Dir(projectDir)
				result = cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				if result.Error != nil {
					return result.Error
				}

				// 8. Verify the manifest contains the aliases
				manifestPath := filepath.Join(ctx.HomeDir(), ".grove", "notebooks", "nb", "workspaces", "test-project", "concepts", "architecture-overview", "concept-manifest.yml")
				manifestContent, err := fs.ReadString(manifestPath)
				if err != nil {
					return err
				}

				return ctx.Verify(func(v *verify.Collector) {
					v.Contains("manifest contains plan alias", manifestContent, "test-project:plans/my-test-plan")
					v.Contains("manifest contains note alias", manifestContent, noteAlias)
				})
			}),
		},
	)
}

// NotebookConceptContextResolutionScenario verifies grove-context can resolve concept aliases.
func NotebookConceptContextResolutionScenario() *harness.Scenario {
	return harness.NewScenario(
		"notebook-concept-context-resolution",
		"Verifies that grove-context can resolve @concept directives with aliased resources.",
		[]string{"notebook", "concepts", "context", "integration"},
		[]harness.Step{
			harness.NewStep("Create concept with aliases and verify cx resolution", func(ctx *harness.Context) error {
				// 1. Setup test project first to get its path
				projectDir := ctx.NewDir("test-project")

				// 2. Setup global config with groves search path
				globalYAML := fmt.Sprintf(`
version: "1.0"
groves:
  e2e-test-projects:
    path: "%s"
notebooks:
  rules:
    default: "main"
  definitions:
    main:
      root_dir: "~/.grove/notebooks/nb"
`, filepath.Dir(projectDir))
				globalConfigDir := filepath.Join(ctx.HomeDir(), ".config", "grove")
				if err := fs.CreateDir(globalConfigDir); err != nil {
					return fmt.Errorf("failed to create global config dir: %w", err)
				}
				if err := fs.WriteString(filepath.Join(globalConfigDir, "grove.yml"), globalYAML); err != nil {
					return err
				}

				// 3. Setup project configuration
				if err := fs.WriteString(filepath.Join(projectDir, "grove.yml"), "name: test-project\nversion: '1.0'"); err != nil {
					return err
				}
				if _, err := git.SetupTestRepo(projectDir); err != nil {
					return err
				}

				// 4. Create a plan with recognizable content in the centralized notebook
				// Plans are stored in the notebook location, not the workspace
				plansDir := filepath.Join(ctx.HomeDir(), ".grove", "notebooks", "nb", "workspaces", "test-project", "plans", "architecture-plan")
				if err := fs.CreateDir(plansDir); err != nil {
					return err
				}
				planContent := "# Architecture Plan\n\nThis plan describes the architecture MARKER_PLAN_CONTENT."
				if err := fs.WriteString(filepath.Join(plansDir, "01-spec.md"), planContent); err != nil {
					return err
				}

				// 5. Create a note with recognizable content
				cmd := ctx.Bin("new", "--no-edit", "architecture note").Dir(projectDir)
				result := cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				if result.Error != nil {
					return result.Error
				}

				// Find and update the note with recognizable content
				notesDir := filepath.Join(ctx.HomeDir(), ".grove", "notebooks", "nb", "workspaces", "test-project", "inbox")
				noteFiles, err := fs.ListFiles(notesDir)
				if err != nil {
					return fmt.Errorf("failed to list note files: %w", err)
				}
				if len(noteFiles) != 1 {
					return fmt.Errorf("expected 1 note file, got %d", len(noteFiles))
				}
				noteFilename := filepath.Base(noteFiles[0])
				notePath := filepath.Join(notesDir, noteFilename)

				// Append recognizable content to the note
				existingContent, err := fs.ReadString(notePath)
				if err != nil {
					return err
				}
				noteContent := existingContent + "\n\nThis note contains MARKER_NOTE_CONTENT for testing."
				if err := fs.WriteString(notePath, noteContent); err != nil {
					return err
				}

				// 6. Create a concept
				cmd = ctx.Bin("concept", "new", "System Architecture").Dir(projectDir)
				result = cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				if result.Error != nil {
					return result.Error
				}

				// 7. Add recognizable content to concept overview
				conceptPath := filepath.Join(ctx.HomeDir(), ".grove", "notebooks", "nb", "workspaces", "test-project", "concepts", "system-architecture")
				overviewPath := filepath.Join(conceptPath, "overview.md")
				existingOverview, err := fs.ReadString(overviewPath)
				if err != nil {
					return err
				}
				overviewContent := existingOverview + "\n\nThis concept overview contains MARKER_CONCEPT_CONTENT."
				if err := fs.WriteString(overviewPath, overviewContent); err != nil {
					return err
				}

				// 8. Link the plan and note using aliases
				cmd = ctx.Bin("concept", "link", "plan", "system-architecture", "test-project:plans/architecture-plan").Dir(projectDir)
				result = cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				if result.Error != nil {
					return result.Error
				}

				noteAlias := fmt.Sprintf("test-project:inbox/%s", noteFilename)
				cmd = ctx.Bin("concept", "link", "note", "system-architecture", noteAlias).Dir(projectDir)
				result = cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				if result.Error != nil {
					return result.Error
				}

				// 9. Create a rules file with @concept directive
				rulesContent := "@concept: system-architecture\n"
				rulesPath := filepath.Join(projectDir, ".context")
				if err := fs.WriteString(rulesPath, rulesContent); err != nil {
					return err
				}

				// 10. Run cx to resolve the concept
				// Find the cx binary - check multiple locations
				cxBinary, err := exec.LookPath("cx")
				if err != nil {
					// Try common installation locations
					homeBin := filepath.Join(ctx.HomeDir(), ".grove", "bin", "cx")
					if fs.Exists(homeBin) {
						cxBinary = homeBin
					} else {
						// Try the real home directory (test sandbox uses fake home)
						realHome, _ := os.UserHomeDir()
						realHomeBin := filepath.Join(realHome, ".grove", "bin", "cx")
						if fs.Exists(realHomeBin) {
							cxBinary = realHomeBin
						} else {
							return fmt.Errorf("cx binary not found, please install grove-context")
						}
					}
				}

				// First set the rules to resolve the concept
				cxSetRulesCmd := ctx.Command(cxBinary, "set-rules", ".context").Dir(projectDir)
				cxSetRulesResult := cxSetRulesCmd.Run()
				ctx.ShowCommandOutput(cxSetRulesCmd.String(), cxSetRulesResult.Stdout, cxSetRulesResult.Stderr)
				if cxSetRulesResult.Error != nil {
					return cxSetRulesResult.Error
				}

				// Then generate the context file
				cxGenCmd := ctx.Command(cxBinary, "generate").Dir(projectDir)
				cxGenResult := cxGenCmd.Run()
				ctx.ShowCommandOutput(cxGenCmd.String(), cxGenResult.Stdout, cxGenResult.Stderr)
				if cxGenResult.Error != nil {
					return cxGenResult.Error
				}

				// 11. Read the generated context file
				contextPath := filepath.Join(projectDir, ".grove", "context")
				contextContent, err := fs.ReadString(contextPath)
				if err != nil {
					return fmt.Errorf("failed to read context file: %w", err)
				}

				// Note: The @concept directive only includes the concept's overview.md file.
				// Linked plans and notes are NOT automatically expanded in the context.
				// This test verifies the basic @concept resolution works.
				return ctx.Verify(func(v *verify.Collector) {
					v.Contains("context includes concept overview", contextContent, "MARKER_CONCEPT_CONTENT")
					// TODO: If cx is updated to expand linked resources, add these checks:
					// v.Contains("context includes plan content", contextContent, "MARKER_PLAN_CONTENT")
					// v.Contains("context includes note content", contextContent, "MARKER_NOTE_CONTENT")
				})
			}),
		},
	)
}
