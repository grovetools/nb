# Neovim Integration

The `nb.nvim` plugin provides a cohesive experience for managing `grove-notebook` directly within Neovim. It is designed to integrate with the developer's workflow, making it simple to create, find, and manage notes without leaving the editor.

## Installation

The plugin is intended to be installed locally from the `grove-notebook` repository.

**Using `lazy.nvim`:**

```lua
{
  -- Assumes grove-notebook is cloned locally
  dir = "~/path/to/grove-notebook/nvim-plugin",
  dependencies = {
    'stevearc/oil.nvim', -- Optional, for directory browsing
  },
  config = function()
    require('nb').setup({
      -- Path to your nb binary if it's not in your shell's PATH
      nb_command = "nb",
    })
  end,
}
```

## Key Features

-   **Context-Aware Note Creation**: Use a picker to create new notes of any type (`current`, `llm`, `issues`, etc.). The plugin automatically determines the correct workspace and branch context.
-   **Advanced Note Searching**: The plugin includes several Snack-powered pickers for different search scopes, all with live previews:
    -   Search global notes (`<leader>ng`).
    -   Search all notes from all branches within the current repository (`<leader>nb`).
-   **Interactive Management**: Archive notes directly from any search picker using a keybinding (`<C-x>`), with support for multi-selection.

## Commands and Keybindings

The plugin exposes both Vim commands and configurable key mappings for common actions.

### Default Keybindings

| Keybinding      | Action                                                   |
| --------------- | -------------------------------------------------------- |
| `<leader>nn`    | Create a new note (opens a type selection picker).       |
| `<leader>ns`    | Search for notes in the current repository.              |
| `<leader>ng`    | Search for global notes.                                 |
| `<leader>na`    | Search for all notes across all workspaces.              |
| `<leader>nb`    | Search for all notes across all branches in this repository. |
| `<leader>nr`    | Archive the current note or notes older than 30 days.    |
| `<leader>ni`    | Open the directory for `current` notes.                  |
| `<leader>nc`    | Open the directory for `llm` (chat) notes.               |
| `<leader>nl`    | Open the directory for `learn` notes.                    |

### Vim Commands

| Command                 | Description                                    |
| ----------------------- | ---------------------------------------------- |
| `:NbNew [title]`        | Creates a new note.                            |
| `:NbQuick "content"`    | Creates a quick, one-line note.                |
| `:NbList [type]`        | Lists notes of a specific type in the quickfix window. |
| `:NbSearch <query>`     | Searches notes and populates the quickfix window. |
| `:NbArchive [file]`     | Archives the specified note or the current note. |
| `:NbContext`            | Displays the current workspace context.        |

## Grove Flow Integration

`grove-notebook` serves as the persistent storage layer for `grove-flow` plans and chats, and the Neovim plugin streamlines this interaction. The `llm` note type in `nb.nvim` directly corresponds to the chat files used by `grove-flow`.

### Managing Plans and Chats

-   **Start a New Chat**: Run `<leader>nn` and select the `llm` note type. This creates a new Markdown file in the appropriate directory, ready to be used with the `flow chat run` command.
-   **Browse Existing Plans**: Use the search pickers (`<leader>ns` or `<leader>na`) to find and open chat or plan files. The live preview makes it easy to review job prompts and outputs without leaving Neovim.


## Full Configuration Example

The plugin's mappings and behavior can be customized in the `setup` function.

```lua
require('nb').setup({
  -- Use a custom path if the 'nb' binary is not in your PATH
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
