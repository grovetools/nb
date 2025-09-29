This document provides a series of practical examples to demonstrate how to use `grove-notebook` (`nb`) in common developer workflows, from basic note-taking to integration with the broader Grove ecosystem.

## Example 1: Basic Note-Taking from the CLI

This example covers the fundamental command-line workflow for creating, finding, and organizing your notes.

1.  **Initialize `nb` and Your Workspace**

    First, set up the global `nb` configuration and note directories. Then, navigate to your project directory and register it as a workspace.

    ```bash
    # Run this once to set up global directories (~/Documents/nb)
    nb init

    # Navigate to your git repo and register it
    cd ~/projects/my-api
    nb workspace add .
    ```

2.  **Create Different Types of Notes**

    `nb` organizes notes by type, which correspond to subdirectories. You can create notes for different purposes using the `-t` flag.

    ```bash
    # Create an architecture note
    nb new "Initial API Design" -t architecture

    # Create a note to track a bug
    nb new "User login fails with special characters" -t issues/bug

    # Create a quick, one-line note without opening an editor
    nb quick "Remember to review PR #114"
    ```

    These commands create files in structured paths, such as:
    `~/Documents/nb/repos/my-api/main/architecture/20250926-initial-api-design.md`
    `~/Documents/nb/repos/my-api/main/issues/bug/20250926-user-login-fails-with-special-characters.md`
    `~/Documents/nb/repos/my-api/main/quick/20250926-150405-quick.md`

3.  **List and Search Your Notes**

    Once you have notes, you can easily list or search them. The search is powered by SQLite FTS5 for fast, full-text queries.

    ```bash
    # List all notes in the current workspace and branch
    nb list --all

    # Search for notes containing "api" in the current workspace
    nb search "api"

    # Search for notes containing "login" across ALL workspaces
    nb search "login" --workspaces
    ```

4.  **Manage Notes with the TUI**

    For a more visual way to browse, filter, and perform bulk actions like archiving, use the interactive Terminal UI (TUI).

    ```bash
    nb manage
    ```

    This launches an interface where you can navigate all notes in the current context, see their types and modification dates, and archive multiple notes at once.

## Example 2: Neovim Integration

The `nb.nvim` plugin provides a note-taking experience directly from Neovim.

1.  **Create a New Note with the Picker**

    `<leader>nn` opens a note type selection picker. You can choose a type, such as "todos", enter a title like "Refactor User Model", and `nb` will create the note and open it in a new buffer, ready for editing.

2.  **Search for Notes with Snacks.nvim**

    Need to reference a previous design decision? Press `<leader>ns` to open a picker that searches all notes in the current repository. As you type, the list filters in a preview panel shows the content of the selected note.

3.  **Create a Global Note**

    Sometimes you need to jot down a note that isn't tied to a specific project, like a new terminal command you learned.
    -   Press `<leader>nng` to open the Global type picker.
    -   A second picker appears for global note types (e.g., "learn", "quick").
    -   Select "learn", enter a title like "jq command for parsing JSON", and the note is saved to your global notebook, accessible from any workspace.

## Example 3: Grove Flow Integration

`grove-notebook` can serve as the persistent storage backend for `grove-flow`. This integration creates a unified, searchable knowledge base for your development notes, AI-driven plans, and chat sessions.

1.  **Configure the Notebook Directory**

    In your global `~/.config/grove/grove.yml`, you define a central `notebook_dir`. This tells all Grove tools where to store long-term artifacts.

    ```yaml
    # ~/.config/grove/grove.yml
    notebook_dir: ~/Documents/nb
    ```

2.  **Initialize a Flow Plan**

    When you're in a project and initialize a `flow` plan, it is automatically stored within your notebook, scoped to the current project and branch.

    ```bash
    # In your project directory ~/projects/my-api on the 'main' branch
    flow plan init api-refactor --recipe standard-feature
    ```

3.  **Locate the Plan in Your Notebook**

    The `api-refactor` plan is now stored within your `nb` directory structure. 

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

```
