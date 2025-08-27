# grove notebook

![grove-notebook](https://github.com/user-attachments/assets/e43f97b0-9d06-48ad-aa41-29e3a94fb499)


[![CI](https://github.com/mattsolo1/grove-notebook/actions/workflows/ci.yml/badge.svg)](https://github.com/mattsolo1/grove-notebook/actions/workflows/ci.yml)

`nb` is a command-line note-taking system for use with the the grove LLM coding ecosystem. It organizes notes based on project workspaces, integrates with Git branches, and provides search capabilities through SQLite. It's built for the command line and integrates with Neovim and Obsidian.

## Features

-   **Workspace-Aware**: Automatically associates notes with your project directories.
-   **Git Integration**: Scopes notes to your current Git branch, keeping feature work organized.
-   **Full-Text Search**: Instant and powerful search across all notes with SQLite FTS5.
-   **Rich CLI**: A comprehensive set of commands for managing notes, workspaces, and more.
-   **Interactive TUI**: A terminal UI (`nb manage`) for browsing and bulk-managing notes.
-   **Editor Integration**: Seamless experience with official [Neovim](#neovim) and [Obsidian](#obsidian) plugins.
-   **Data Integrity Tools**: `nb migrate` and `nb doctor` help standardize and repair your notes.
-   **Flexible Note Types**: Organize notes by category (`current`, `llm`, `learn`, `issues`, etc.) with support for custom nested types.

## Installation

**Note**: The build process requires SQLite with the FTS5 extension enabled, which is standard on most modern systems.

## Quick Start

1.  **Initialize `nb`:**
    This sets up the global configuration and notebook directory (`~/.local/share/nb` and `~/Documents/nb`).
    ```bash
    nb init
    ```

2.  **Register a workspace:**
    Navigate to your project directory and register it.
    ```bash
    cd ~/projects/my-project
    nb workspace add .
    ```

3.  **Create your first note:**
    This creates a new note and opens it in your default `$EDITOR`.
    ```bash
    nb new "API design ideas"
    ```

4.  **List your notes:**
    ```bash
    nb list
    ```

5.  **Search your notes:**
    ```bash
    nb search "api"          # Search current workspace
    nb search "api" --all    # Search all workspaces
    ```

## Command Overview

`nb` provides a rich set of commands for all your note-taking needs.

### Core Commands

| Command           | Description                                                        |
| ----------------- | ------------------------------------------------------------------ |
| `nb new [title]`  | Create a new note. Use `-t <type>` for different categories.       |
| `nb quick "..."`  | Create a quick, one-line note without opening an editor.           |
| `nb list [type]`  | List notes. Flags: `--all`, `--workspaces`, `--all-branches`.      |
| `nb search <q>`   | Search notes using SQLite FTS5.                                    |
| `nb archive`      | Archive notes, either by name or with `--older-than <days>`.       |
| `nb manage`       | Open an interactive terminal UI to browse and manage notes.        |

### Workspace & Context

| Command                  | Description                                            |
| ------------------------ | ------------------------------------------------------ |
| `nb init`                | Initializes the `nb` configuration and directories.    |
| `nb workspace add [path]`| Registers a directory as a workspace.                  |
| `nb workspace list`      | Lists all registered workspaces.                       |
| `nb workspace remove`    | Removes a workspace registration.                      |
| `nb context`             | Shows information about the current workspace context. |

### Utility Commands

| Command                  | Description                                                      |
| ------------------------ | ---------------------------------------------------------------- |
| `nb move <src> <dest>`   | Move notes between types, branches, or workspaces.               |
| `nb migrate`             | Standardize frontmatter, filenames, and tags of existing notes.  |
| `nb doctor`              | Check for and fix common configuration issues.                   |
| `nb version`             | Show version information.                                        |

## Editor Integration

### Neovim

The official `nb.nvim` plugin provides a seamless integration with Neovim.

**Installation (lazy.nvim):**

```lua
{
  dir = "~/path/to/grove-notebook/nvim-plugin",
  config = function()
    require('nb').setup({
      -- path to your nb binary if not in PATH
      nb_command = "nb",
    })
  end,
}
```

**Key Mappings:**

-   `<leader>nn`: Create a new note (with a type selection picker).
-   `<leader>ns`: Search notes in the current repository.
-   `<leader>ng`: Search global notes.
-   `<leader>na`: Search all notes across all workspaces.
-   For a full list of features and mappings, see the [plugin's README](./nvim-plugin/README.md).

### Obsidian

An experimental Obsidian plugin is available for development and testing. It provides a sidebar view to browse your `nb` repositories and notes. See the [plugin's README](./obsidian-plugin/README.md) for setup instructions.

## Development

-   **Build:** `make build`
-   **Test:** `make test`
-   **Lint:** `make lint` (requires `golangci-lint`)

All common development tasks are defined in the `Makefile`.
