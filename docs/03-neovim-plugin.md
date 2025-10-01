# Neovim Integration

The `nb.nvim` plugin provides functions for managing `grove-notebook` files within Neovim. It uses the `nb` command-line tool to determine the correct workspace, branch, and file paths for note operations.

## Installation

The plugin is installed from a local clone of the `grove-notebook` repository.

**Using `lazy.nvim`:**

```lua
{
  -- Path to the local grove-notebook repository
  dir = "~/path/to/grove-notebook/nvim-plugin",
  dependencies = {
    'stevearc/oil.nvim', -- Optional, for directory browsing
  },
  config = function()
    require('nb').setup({
      -- Path to the nb binary if not in shell's PATH
      nb_command = "nb",
    })
  end,
}
```

## Features

*   **Note Creation**: A picker interface allows creating new notes. The plugin determines the correct storage path based on the current workspace and Git branch context.
*   **Note Searching**: The plugin provides several pickers for searching notes, each with a live preview:
    *   Search within the current repository.
    *   Search global notes, which are not associated with a repository.
    *   Search all notes across all registered workspaces.
    *   Search all notes from all branches within the current repository.
*   **Note Management**: Notes can be archived from any search picker, with support for selecting multiple notes.

## Commands and Keybindings

The plugin provides Vim commands and a set of default key mappings.

### Default Keybindings

| Keybinding | Action |
| --- | --- |
| `<leader>nn` | Opens a picker to select a note type and create a new note. |
| `<leader>ns` | Searches for notes in the current repository. |
| `<leader>ng` | Searches for global notes. |
| `<leader>na` | Searches for all notes across all workspaces. |
| `<leader>nb` | Searches for all notes from all branches in the current repository. |
| `<leader>nr` | Archives the current note or notes older than 30 days. |
| `<leader>ni` | Opens the directory for `current` notes. |
| `<leader>nc` | Opens the directory for `llm` (chat) notes. |
| `<leader>nl` | Opens the directory for `learn` notes. |

### Vim Commands

| Command | Description |
| --- | --- |
| `:NbNew [title]` | Creates a new note. |
| `:NbQuick "content"` | Creates a quick, one-line note. |
| `:NbList [type]` | Lists notes of a specific type in the quickfix window. |
| `:NbSearch <query>` | Searches notes and populates the quickfix window. |
| `:NbArchive [file]` | Archives the specified note or the current note. |
| `:NbContext` | Displays the current workspace context. |

## Grove Flow Integration

`grove-notebook` can be configured as the storage location for `grove-flow` plans and chats. The `nb.nvim` plugin creates files that can then be used by `grove-flow`.

### Managing Plans and Chats

*   **Start a New Chat**: Running the create note command (`<leader>nn`) and selecting the `llm` note type creates a new Markdown file. This file is formatted as a `grove-flow` chat job and can be executed from the terminal with `flow chat run`.
*   **Browse Existing Plans**: The search pickers (`<leader>ns`, `<leader>na`) can find and open `grove-flow` plan and chat files stored within the notebook directory. The live preview shows job prompts and outputs.

## Configuration Example

The plugin's mappings and the path to the `nb` binary can be set in the `setup` function.

```lua
require('nb').setup({
  -- Use a custom path if the 'nb' binary is not in the shell's PATH
  nb_command = vim.fn.expand("~/path/to/grove-notebook/bin/nb"),
  mappings = {
    current = '<leader>ni',       -- Open current notes
    llm = '<leader>nc',           -- Open LLM (chat) notes
    learn = '<leader>nl',         -- Open learning notes
    new = '<leader>nn',           -- Create a new note with a picker
    path = '<leader>cp',          -- Copy the current file path
    search = '<leader>ns',        -- Search notes in the current repository
    search_global = '<leader>ng', -- Search global notes
    search_all = '<leader>na',    -- Search all notes across all workspaces
    search_repo = '<leader>nb',   -- Search all notes in the current repository
    archive = '<leader>nr',       -- Archive notes
    -- Disable a mapping by setting it to false
    move = false,
  },
})
```