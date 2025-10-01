# Grove Notebook

<img src="./images/grove-notebook-inkscape.svg" width="60%" />

`grove-notebook` (`nb`) is a command-line note-taking system that organizes notes in Markdown files based on project workspaces and Git branches.

<!-- placeholder for animated gif -->

## Central Knowledge Base

The main notebook directory (default: `~/Documents/nb`) is intended to be stored outside of any specific Git repository. This design allows notes to persist across different projects, creating a central knowledge base that is not tied to a single repository's lifecycle.

## Key Features

*   **Note Organization**: Notes are stored as Markdown files.
*   **Search**: Provides full-text search capabilities using a SQLite FTS5 index.
*   **`grove-flow` Storage**: Can be configured as a storage location for `grove-flow` plans and chat sessions.
*   **Editor Integration**: Includes a Neovim plugin for creating and searching notes.
*   **Obsidian Compatibility**: The notebook directory can be opened as an Obsidian vault.
*   **CLI**: A command-line interface for creating and managing notes.

## How It Works

`nb` uses a SQLite database (`workspaces.db`) in its data directory (`~/.local/share/nb`) to register project directories as workspaces. When a command is run, `nb` detects the current workspace by traversing up from the current directory to find a registered path.

Notes are stored in a hierarchical directory structure, typically `NOTEBOOK_DIR/TYPE/WORKSPACE_NAME/BRANCH_NAME/NOTE_TYPE/`. For example, a note of type `current` for the `main` branch of the `my-api` repository would be stored in `~/Documents/nb/repos/my-api/main/current/`. A SQLite database (`index.db`) is used to index notes for search.

## Ecosystem Integration

`grove-notebook` can function as a storage backend for other tools in the Grove ecosystem.

*   **`grove-flow`**: The `plans_directory` and `chat_directory` in `grove-flow`'s configuration can be set to paths within the `nb` directory structure. This stores all `flow` plans and chat logs in the central notebook.
*   **Neovim Plugin**: The `nb.nvim` plugin provides commands to create notes and execute searches from within the editor. It can also be used to create files that are then used by `grove-flow` commands.

## Installation

Install via the Grove meta-CLI:
```bash
grove install nb
```

Verify installation:
```bash
nb version
```

Requires the `grove` meta-CLI. See the [Grove Installation Guide](https://github.com/mattsolo1/grove-meta/blob/main/docs/02-installation.md) if you don't have it installed.
