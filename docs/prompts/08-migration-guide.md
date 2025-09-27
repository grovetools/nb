# Documentation Task: Migrating Existing Notes

You are an expert technical writer. Create a detailed guide for the `nb migrate` command, using `cmd/migrate.go` and the `pkg/migration/` directory as your primary sources.

**Task:**
1.  **Purpose**: Explain why a user would need `nb migrate` (to import and standardize an existing collection of Markdown notes into the `nb` format).
2.  **What it Does**: Detail the specific actions the command performs:
    - Adds or fixes YAML frontmatter (`id`, `title`, `tags`, `created`, `modified`).
    - Standardizes filenames to the `YYYYMMDD-title.md` format.
    - Generates tags from the directory structure.
    - Preserves original file modification times.
3.  **Usage and Options**:
    - Explain the `--dry-run` flag for previewing changes.
    - Describe the fixing flags (`--fix-titles`, `--fix-filenames`, etc.) and the convenience `--all` flag.
    - Detail the scope flags for targeting notes: `--workspace`, `--global`, and `--all-workspaces`.
4.  **Example Workflow**: Provide a step-by-step example of migrating a folder of notes.
    - 1. Copy external notes into a workspace directory (e.g., `~/Documents/nb/repos/my-project/main/current`).
    - 2. Run `nb migrate --dry-run` to see the proposed changes.
    - 3. Run `nb migrate --all` to apply the changes.