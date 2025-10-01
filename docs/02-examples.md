# Examples

This document provides examples for using `grove-notebook` (`nb`), from command-line use to integration with other development tools.

### Example 1: Command-Line Workflow

This example covers the fundamental commands for creating, finding, and organizing notes.

1.  **Initialize `nb` and Register a Workspace**

    First, run `nb init` to set up global note directories. Then, navigate to a project directory and register it as a workspace.

    ```bash
    # Run once to set up global directories (e.g., ~/Documents/nb)
    nb init

    # Navigate to a git repository and register it as a workspace
    cd ~/projects/my-api
    nb workspace add .
    ```

2.  **Create Notes by Type**

    The `-t` flag organizes notes into subdirectories by type. Nested types like `issues/bug` create nested directories.

    ```bash
    # Create an architecture note
    nb new "Initial API Design" -t architecture

    # Create a note for a bug report
    nb new "User login fails with special characters" -t issues/bug

    # Create a quick, one-line note without opening an editor
    nb quick "Remember to review PR #114"
    ```

    These commands create files in structured paths, such as `~/Documents/nb/repos/my-api/main/architecture/YYYYMMDD-initial-api-design.md`.

3.  **List and Search Notes**

    The `list` and `search` commands query the note database. The search is powered by SQLite FTS5 for full-text queries.

    ```bash
    # List all notes in the current workspace and branch
    nb list --all

    # Search for notes containing "api" across ALL workspaces
    nb search "api" --workspaces
    ```

4.  **Manage Notes in the TUI**

    The `nb manage` command starts a terminal interface for browsing, filtering, and performing bulk actions like archiving.

    ```bash
    nb manage
    ```

### Example 2: Neovim Integration

The `nb.nvim` plugin integrates `grove-notebook` into Neovim.

1.  **Create a New Note**

    Pressing `<leader>nn` opens a picker to select a note type. After choosing a type (e.g., "todos") and entering a title ("Refactor User Model"), `nb` creates the note and opens it in a new buffer.

2.  **Search for Notes**

    Pressing `<leader>ns` opens a picker that searches all notes in the current repository. As you type, the list filters and a preview panel shows the content of the selected note.

3.  **Create a Global Note**

    For notes not tied to a specific project, `<leader>nng` opens a picker for global note types. Selecting a type (e.g., "learn") and entering a title creates a note in the global notebook, which is accessible from any workspace.

### Example 3: Grove Flow Integration

`grove-notebook` can serve as the persistent storage backend for `grove-flow`.

1.  **Configure the Notebook Directory**

    In your global `~/.config/grove/grove.yml`, define a central `notebook_dir`. This tells Grove tools where to store long-term artifacts.

    ```yaml
    # ~/.config/grove/grove.yml
    notebook_dir: ~/Documents/nb
    ```

2.  **Initialize a Flow Plan**

    When inside a project directory, initializing a `flow` plan stores it within the notebook, scoped to the current project and branch.

    ```bash
    # In your project directory ~/projects/my-api on the 'main' branch
    flow plan init api-refactor --recipe standard-feature
    ```

3.  **Locate the Plan in Your Notebook**

    The `api-refactor` plan is stored within the `nb` directory structure, making it a searchable part of the knowledge base.

    ```tree
    ~/Documents/nb/
    └── repos/
        └── my-api/
            └── main/
                └── plans/
                    └── api-refactor/
                        ├── 01-spec.md
                        ├── 02-implement.md
                        └── ...
    ```

### Example 4: Obsidian Integration

The `nb` notebook directory can be opened as an Obsidian vault for graphical interaction.

1.  **Open Vault in Obsidian**

    In Obsidian, use the "Open folder as vault" option and select your `nb` directory (e.g., `~/Documents/nb`).

2.  **Combined Workflow**

    This setup allows for a workflow that combines the CLI and a graphical interface.

    -   **Capture**: Use `nb quick "Refactor the auth middleware"` to capture an idea from the terminal without breaking focus.
    -   **Organize**: Later, open Obsidian. The new note appears automatically. You can then link it to other notes, add it to a Kanban board, or expand on it.

3.  **Visualization**

    Use Obsidian's graph view to see connections between notes created across different projects and branches.
