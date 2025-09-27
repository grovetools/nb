# Editor Integrations

`nb` provides first-class integration with popular editors, bringing powerful note-taking capabilities directly into your development environment. This guide covers the setup and usage of the official Neovim and Obsidian plugins.

## Neovim Integration (nb.nvim)

The `nb.nvim` plugin provides seamless integration between Neovim and `nb`, allowing you to create, search, and manage notes without leaving your editor.

### Key Features

- **Quick Note Creation**: Type-aware note creation with visual picker
- **Live Search**: Browse and search notes with instant preview
- **Oil.nvim Integration**: Navigate notes using the familiar oil.nvim file browser
- **Context Awareness**: Automatically detects current workspace and branch
- **Archive Management**: Clean up old notes directly from Neovim
- **Multi-scope Search**: Search current repo, global notes, or everything

### Installation

#### Using lazy.nvim (Recommended)

```lua
{
  dir = "~/path/to/grove-notebook/nvim-plugin",
  dependencies = {
    'stevearc/oil.nvim',     -- Optional but recommended for browsing
    'nvim-telescope/telescope.nvim',  -- Optional for enhanced search
  },
  config = function()
    require('nb').setup({
      -- Path to your nb binary (if not in PATH)
      nb_command = "nb",
      
      -- Custom key mappings (these are the defaults)
      mappings = {
        current = '<leader>ni',        -- Open current notes
        llm = '<leader>nc',           -- Open LLM notes  
        learn = '<leader>nl',         -- Open learning notes
        new = '<leader>nn',           -- Create new note (with picker)
        path = '<leader>cp',          -- Copy file path
        search = '<leader>ns',        -- Search current repository
        search_global = '<leader>ng', -- Search global notes
        search_all = '<leader>na',    -- Search all notes
        search_repo = '<leader>nb',   -- Search all branches in repo
        archive = '<leader>nr',       -- Archive old notes
      },
    })
  end,
}
```

#### Manual Installation

1. Clone the plugin:
   ```bash
   git clone https://github.com/mattsolo1/grove-notebook.git
   cd grove-notebook/nvim-plugin
   ```

2. Add to your Neovim configuration:
   ```lua
   vim.opt.runtimepath:append("~/path/to/grove-notebook/nvim-plugin")
   require('nb').setup()
   ```

### Default Key Mappings

| Mapping | Command | Description |
|---------|---------|-------------|
| `<leader>nn` | Create new note | Opens type selection picker |
| `<leader>ns` | Search repo notes | Search current repository with preview |
| `<leader>ng` | Search global | Search global notes with preview |
| `<leader>na` | Search all | Search across all workspaces |
| `<leader>nb` | Search branches | Search all branches in current repo |
| `<leader>ni` | Browse current | Open current notes in oil.nvim |
| `<leader>nc` | Browse LLM | Open LLM/chat notes |
| `<leader>nl` | Browse learn | Open learning notes |
| `<leader>nr` | Archive notes | Archive old notes interactively |
| `<leader>cp` | Copy path | Copy current file path to clipboard |

#### Quick Creation Shortcuts

| Mapping | Command | Description |
|---------|---------|-------------|
| `<leader>nnl` | New LLM note | Quick create LLM note |
| `<leader>nnd` | New daily note | Quick create daily note |
| `<leader>nnr` | New learn note | Quick create learning note |
| `<leader>nng` | New global note | Quick create global note |

### Search Pickers

The plugin provides multiple search pickers with different scopes:

#### Repository Search (`<leader>ns`)
- Shows all notes in the current repository
- Displays date, type, and title
- Live preview of note contents
- Sorted by date (newest first)

#### Global Search (`<leader>ng`)
- Shows all notes in the global workspace
- Repository-independent notes
- Perfect for personal todos and references

#### All Notes Search (`<leader>na`)
- Combined view of ALL notes across ALL repositories
- Shows repository/branch context
- Searches your entire knowledge base
- Displays results in a table format:
  ```
  Repository    Date        Type      Title
  my-project    2024-01-15  [current] API Design
  blog          2024-01-14  [draft]   New Post Ideas
  global        2024-01-13  [todos]   Weekly Tasks
  ```

#### Repository-wide Search (`<leader>nb`)
- Searches all branches in the current repository
- Useful for finding notes from feature branches
- Shows branch name alongside each result

### Note Creation Picker

When you press `<leader>nn`, a visual picker appears with note type options:

```
┌─ Select Note Type ─────────────┐
│ 📝 Current - General notes     │
│ 🤖 LLM - Chat/AI sessions      │
│ 📚 Learn - Study notes         │
│ 📅 Daily - Journal entries     │
│ 🌍 Global - Personal notes     │
└────────────────────────────────┘
```

Select with arrow keys or type the first letter to jump to an option.

### Oil.nvim Integration

If you have oil.nvim installed, the plugin provides enhanced file browsing:

```lua
-- Browse current notes
vim.keymap.set('n', '<leader>ni', function()
  require('nb').open_in_oil('current')
end)
```

This opens oil.nvim at the appropriate directory with full file management capabilities:
- Create, rename, delete files
- Navigate with familiar vim motions
- Preview files before opening
- Bulk operations support

### Custom Configuration

#### Change Key Mappings

```lua
require('nb').setup({
  mappings = {
    new = '<leader>wn',        -- Different prefix
    search = '<leader>ws',
    search_all = '<leader>wa',
  },
})
```

#### Disable Specific Mappings

```lua
require('nb').setup({
  mappings = {
    archive = false,  -- Disable archive mapping
    path = false,     -- Disable path copy
  },
})
```

#### Custom Note Types

```lua
-- Create a custom command for project-specific note types
vim.api.nvim_create_user_command('NBBug', function()
  vim.fn.system('nb new -t issues/bugs')
end, {})
```

### Telescope Integration

If you use Telescope, the plugin automatically enhances search with:
- Fuzzy finding
- Advanced filtering
- Custom actions
- Better previews

### Tips and Tricks

1. **Quick Note Capture**: Use `<leader>nng` to quickly capture thoughts without context
2. **Project Notes**: Keep `<leader>ns` handy for project-specific searches
3. **Learning Workflow**: Use `<leader>nnr` during coding sessions to capture learnings
4. **Daily Reviews**: Map a key to open today's daily note:
   ```lua
   vim.keymap.set('n', '<leader>nd', function()
     vim.fn.system('nb new -t daily "Daily Note"')
   end)
   ```

## Obsidian Integration

The Obsidian plugin provides a graphical interface for browsing and managing `nb` notes within Obsidian's environment.

### Status

⚠️ **Experimental**: The Obsidian plugin is currently in development and may have limited functionality compared to the Neovim plugin.

### Features

- **Repository Browser**: Navigate repositories and branches from a sidebar
- **Table View**: Clean, organized display of notes
- **Quick Actions**: Create and archive notes
- **Sync Integration**: Automatic synchronization with `nb` repositories
- **Markdown Preview**: View notes with Obsidian's rich markdown rendering

### Installation

#### Development Installation

1. **Navigate to your Obsidian vault's plugins directory:**
   ```bash
   cd /path/to/vault/.obsidian/plugins
   ```

2. **Clone the plugin:**
   ```bash
   git clone https://github.com/mattsolo1/grove-notebook.git nb-plugin
   cd nb-plugin/obsidian-plugin
   ```

3. **Install dependencies and build:**
   ```bash
   npm install
   npm run build
   ```

4. **Enable the plugin:**
   - Open Obsidian Settings
   - Navigate to Community Plugins
   - Enable "NB Integration"

#### Using nb obsidian install-dev

For quick development setup:
```bash
nb obsidian install-dev --vault ~/Documents/MyVault
```

This command:
- Copies plugin files to the vault
- Installs dependencies
- Builds the plugin
- Enables it in Obsidian

### Usage

#### Opening the NB View

1. Click the NB icon in the ribbon (left sidebar)
2. Or use Command Palette: `Cmd/Ctrl+P` → "NB: Open notes view"

#### Browsing Notes

The sidebar view displays:
```
Repository: [my-project ▼]
Branch: [main ▼]

┌─────────────────────────────────┐
│ Date       Type     Title        │
├─────────────────────────────────┤
│ 2024-01-15 current  API Design   │
│ 2024-01-14 issues   Bug Report   │
│ 2024-01-13 learn    Go Patterns  │
└─────────────────────────────────┘
```

#### Creating Notes

1. Click the "+" button in the NB view
2. Select note type from dropdown
3. Enter title
4. Note opens in Obsidian editor

#### Settings

Access plugin settings via Settings → Community Plugins → NB Integration:

| Setting | Description | Default |
|---------|-------------|---------|
| NB Path | Path to nb executable | `nb` |
| Default Type | Default note type for creation | `current` |
| Auto Sync | Sync on vault open | `true` |
| Sync Interval | Minutes between syncs | `10` |

### Development

For plugin development:

```bash
# Watch mode for development
npm run dev

# Production build
npm run build

# Run tests
npm run test
```

### Limitations

Current limitations of the Obsidian plugin:
- Read-only for non-markdown files
- Limited search capabilities (use CLI for advanced search)
- No direct Git integration
- Requires `nb` CLI to be installed

### Roadmap

Planned features for the Obsidian plugin:
- [ ] Advanced search with filters
- [ ] Direct Git operations
- [ ] Template support
- [ ] Bulk operations
- [ ] Graph view integration
- [ ] Mobile support

## Comparison

| Feature | Neovim Plugin | Obsidian Plugin |
|---------|--------------|-----------------|
| Note Creation | ✅ Full support | ✅ Full support |
| Search | ✅ Advanced with preview | ⚠️ Basic |
| Browse | ✅ Oil.nvim integration | ✅ Table view |
| Archive | ✅ Supported | ✅ Supported |
| Git Integration | ✅ Full context awareness | ⚠️ Limited |
| Performance | ✅ Very fast | ✅ Good |
| Mobile | ❌ N/A | 🔄 Planned |

## Integration with Other Editors

While official plugins exist for Neovim and Obsidian, `nb` can be integrated with any editor that supports:
- External command execution
- Custom keybindings
- Terminal integration

### VS Code

Example tasks.json:
```json
{
  "version": "2.0.0",
  "tasks": [
    {
      "label": "New NB Note",
      "type": "shell",
      "command": "nb new '${input:noteTitle}'",
      "problemMatcher": []
    }
  ],
  "inputs": [
    {
      "id": "noteTitle",
      "type": "promptString",
      "description": "Note title"
    }
  ]
}
```

### Emacs

Example configuration:
```elisp
(defun nb-new-note (title)
  "Create a new nb note"
  (interactive "sNote title: ")
  (shell-command (format "nb new '%s'" title)))

(global-set-key (kbd "C-c n n") 'nb-new-note)
```

### Sublime Text

Create a build system:
```json
{
  "cmd": ["nb", "new", "$file_base_name"],
  "working_dir": "$project_path",
  "selector": "source.markdown"
}
```

## Best Practices

1. **Choose the Right Plugin**: Use Neovim for development work, Obsidian for knowledge management
2. **Consistent Keybindings**: Align plugin mappings with your workflow
3. **Template Integration**: Configure editor snippets to match `nb` templates
4. **Search Strategy**: Use repository search for project work, global search for references
5. **Regular Archiving**: Use archive features to keep workspaces clean

## Troubleshooting

### Neovim Plugin Issues

**Plugin not loading:**
```vim
:checkhealth nb
```

**Key mappings conflict:**
```lua
-- Check existing mappings
:verbose nmap <leader>n
```

**nb command not found:**
```lua
require('nb').setup({
  nb_command = "/full/path/to/nb"
})
```

### Obsidian Plugin Issues

**Plugin not visible:**
- Check console for errors: `Ctrl+Shift+I`
- Verify plugin is enabled in settings
- Restart Obsidian

**Sync not working:**
- Verify `nb` is in PATH
- Check plugin settings for correct path
- Review Obsidian console for error messages

## Getting Help

- **Neovim Plugin**: [Issues](https://github.com/mattsolo1/grove-notebook/issues)
- **Obsidian Plugin**: [Issues](https://github.com/mattsolo1/grove-notebook/issues)
- **Documentation**: [Official Docs](https://github.com/mattsolo1/grove-notebook)