# nb.nvim - Neovim Plugin for nb Note System

This plugin provides seamless integration between Neovim and the nb note-taking system.

## Features

- Quick note creation with type selection picker
- Browse notes with oil.nvim integration
- Context-aware paths based on current workspace/branch
- Search across all notes with live preview
- Archive old notes
- Support for different note types (current, llm, learn, daily)
- Global notes that are independent of repository context

## Installation

### Using lazy.nvim

```lua
{
  dir = "~/path/to/nb-prototype/nvim-plugin",
  dependencies = {
    'stevearc/oil.nvim',  -- Optional but recommended
  },
  config = function()
    require('nb').setup({
      nb_command = "nb",  -- Path to nb binary
      mappings = {
        current = '<leader>ni',   -- Open current notes
        llm = '<leader>nc',       -- Open LLM notes
        learn = '<leader>nl',     -- Open learning notes
        new = '<leader>nn',       -- Create new note
        path = '<leader>cp',      -- Copy file path
        search = '<leader>ns',    -- Search notes in current repo
        search_global = '<leader>ng', -- Search global notes
        search_all = '<leader>na', -- Search all notes
        search_repo = '<leader>nb', -- Search all notes across all branches in current repository
        archive = '<leader>nr',   -- Archive old notes
      },
    })
  end,
}
```

## Usage

### Key Mappings

- `<leader>ni` - Open current notes directory in oil.nvim
- `<leader>nc` - Open LLM/chat notes directory
- `<leader>nl` - Open learning notes directory
- `<leader>nn` - Create a new note with type selection picker
- `<leader>nnl` - Create a new LLM note (quick)
- `<leader>nnd` - Create a new daily note (quick)
- `<leader>nnr` - Create a new learning note (quick)
- `<leader>nng` - Create a new global note (quick)
- `<leader>ns` - Search all notes in current repository with preview
- `<leader>ng` - Search all global notes with preview
- `<leader>na` - Search all notes (global + repository) with preview
- `<leader>nb` - Search all notes across all branches in the current repository
- `<leader>nr` - Archive old notes
- `<leader>cp` - Copy current file path to clipboard

### Note Creation Picker

When you press `<leader>nn`, a picker appears with these options:
- 📝 **Current** - General notes for current work
- 🤖 **LLM** - Chat/AI session notes
- 📚 **Learn** - Learning and study notes
- 📅 **Daily** - Daily journal entries
- 🌍 **Global** - General notes (not repo-specific)

### Notes Search Picker

The repository search picker (`<leader>ns`) shows:
- All notes in the current repository context
- Date of creation in YYYY-MM-DD format
- Note type in brackets [current], [llm], etc.
- Live preview of note contents
- Sorted by date (newest first)

### Global Notes Search Picker

The global search picker (`<leader>ng`) shows:
- All notes in the global workspace (repository-independent)
- Same format as repository search
- Useful for personal notes, todos, and reference material

### All Notes Search Picker

The all notes search picker (`<leader>na`) shows:
- Combined view of ALL notes across ALL repositories and branches
- Repository/branch displayed as a column (e.g., `note-system/main`, `backend/feature-xyz`, or `global/-`)
- Note type, date, and title in a table format
- Same preview and sorting features
- Useful for searching across your entire knowledge base, regardless of current context

### Repository-wide Search Picker

The repository-wide search picker (`<leader>nb`) shows:
- All notes from ALL branches in the current repository
- Branch name displayed in parentheses for each note
- Useful when working on a feature branch but needing to reference notes from main or other branches
- Same preview and sorting features as other pickers

### Commands

- `:NbNew [title]` - Create a new note
- `:NbList [type]` - List notes of given type
- `:NbSearch <query>` - Search notes (results in quickfix)
- `:NbArchive [args]` - Archive old notes
- `:NbContext` - Show current workspace context

## Requirements

- Neovim 0.8.0 or later
- nb CLI tool installed and in PATH (or configured with full path)
- oil.nvim (optional, falls back to netrw)
- snacks.nvim (optional, for enhanced picker UI)

## Configuration

All mappings can be customized or disabled by setting them to `false`:

```lua
require('nb').setup({
  nb_command = "nb",  -- or full path like "~/bin/nb"
  mappings = {
    current = '<leader>ni',
    llm = false,  -- Disable this mapping
    -- ... other mappings
  },
})
```

### Full Configuration Example

```lua
require('nb').setup({
  nb_command = vim.fn.expand("~/Code/note-system/nb-prototype/nb"),
  mappings = {
    current = '<leader>ni',   -- Open current notes
    llm = '<leader>nc',       -- Open LLM notes
    learn = '<leader>nl',     -- Open learning notes
    new = '<leader>nn',       -- Create new note with picker
    path = '<leader>cp',      -- Copy file path
    search = '<leader>ns',    -- Search notes with preview
    search_global = '<leader>ng', -- Search global notes
    search_all = '<leader>na', -- Search all notes
    search_repo = '<leader>nb', -- Search all notes across all branches
    archive = '<leader>nr',   -- Archive old notes
  },
})
```
