<p align="center">
  <img src="https://grovetools.ai/docs/nb/images/nb-logo-with-text-dark.svg" alt="Grove Notebook">
</p>

<!-- DOCGEN:OVERVIEW:START -->

`nb` is a command-line tool for managing Markdown notes within a structured, workspace-aware directory system. It creates a centralized storage layer for engineering notes, architectural concepts, and project plans, decoupled from the source code repositories they reference.

## Core Mechanisms

**Workspace Context**: `nb` utilizes the `grove` core discovery service to map the current working directory to a specific workspace. Commands executed in `~/code/my-project` automatically target `~/.grove/notebooks/nb/workspaces/my-project/`, ensuring notes are organized by project context without cluttering the source repository.

**Storage Structure**: Notes are stored as Markdown files with YAML frontmatter. The default hierarchy organizes files by workspace and note type (e.g., `inbox`, `daily`, `plans`).
*   **Frontmatter**: Metadata such as `id`, `tags`, and `created` timestamps are maintained in the file header.
*   **Centralization**: By default, all data resides in `~/.grove/notebooks/`, allowing for a unified knowledge base that can be backed up or version-controlled independently of project code.

**Integration**: Acts as the storage backend for other ecosystem tools. `flow` reads plans from the `plans/` directory and `docgen` can store draft documentation.

## Features

### Note Management
*   **Creation**: `nb new` creates timestamped files in the `inbox` directory of the active workspace. Supports templates based on note type (e.g., `daily` generates a task list structure).
*   **Organization**: Commands like `archive` and `move` manage file lifecycles.
*   **Search**: `nb search` executes `ripgrep` (or `grep`) across the notebook directory, respecting workspace boundaries.

### Terminal Interface (TUI)
`nb tui` launches a file browser for navigating the notebook structure.
*   **Navigation**: Vim-style keybindings for traversing the workspace tree.
*   **Filtering**: Supports filtering by tag (`&`) or content (`/`).
*   **Preview**: Renders Markdown content in a side pane.
*   **Git Status**: Visualizes file status if the notebook directory is a Git repository.

### Concept Management
`nb concept` manages architectural knowledge entities.
*   **Structure**: Creates a directory containing a `concept-manifest.yml` and `overview.md`.
*   **Linking**: Establishes relationships between concepts, plans, and notes via the manifest, enabling a graph-like structure for technical documentation.

### Remote Synchronization
`nb remote sync` synchronizes local Markdown notes with remote issue trackers (currently GitHub Issues and Pull Requests).
*   **Bi-directional Sync**: Updates local files based on remote changes and pushes local edits to the remote provider based on modification timestamps.
*   **Metadata Mapping**: Maps frontmatter fields (`remote.id`, `remote.state`) to GitHub API fields.

### Version Control
`nb git` provides helpers for managing the notebook's own version control.
*   **Initialization**: `nb git init` configures a Git repository in the notebook root or specific workspace directory, generating appropriate `.gitignore` files.
*   **Operations**: `nb git commit` stages and commits changes within the notebook context.

## Integrations

*   **Obsidian**: The directory structure is compatible with Obsidian vaults. The `nb obsidian install-dev` command links local plugins for deeper integration (this is out of date currently).
*   **Editors**: Opens notes in the system default `$EDITOR`.

<!-- DOCGEN:OVERVIEW:END -->

<!-- DOCGEN:TOC:START -->

See the [documentation](docs/) for detailed usage instructions:
- [Overview](docs/01-overview.md)
- [Configuration](docs/02-configuration.md)
- [Command Reference](docs/03-command-reference.md)

<!-- DOCGEN:TOC:END -->
