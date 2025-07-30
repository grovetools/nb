# nb - Workspace-Based Note System Prototype

This is a prototype implementation of the alternative architecture for a workspace-based note-taking system.

## Features

- **Workspace Registry**: No symlinks - workspaces are registered and auto-detected
- **SQLite Search**: Full-text search with FTS5
- **Git Integration**: Branch-aware note organization
- **CLI Tool**: Complete command-line interface
- **Neovim Plugin**: Seamless editor integration

## Quick Start

### Build and Install

```bash
cd nb-prototype

# Option 1: Using Make (recommended)
make build              # Build with FTS5 support
make install            # Build and install to /usr/local/bin

# Option 2: Manual build
go build -tags "fts5" -o nb ./cmd/nb
sudo mv nb /usr/local/bin/  # Or add to PATH

# Initialize
nb init
```

**Important**: The `-tags "fts5"` flag is required for SQLite full-text search support.

### Basic Usage

```bash
# Create notes
nb new                      # Create timestamped note
nb new "meeting notes"      # Create with title
nb new -t llm "chat"       # Create LLM note

# Workspace management
nb workspace add .          # Register current directory
nb workspace list           # List all workspaces
nb workspace current        # Show current workspace

# Search
nb search "authentication"  # Search current workspace
nb search "api" --all       # Search all workspaces

# List and archive
nb list                     # List current notes
nb archive --older-than 30  # Archive old notes
```

### 4. Quick Note Capture

**Impact**: Frictionless note creation

```bash
# Add quick flag for instant notes
$ nb quick "Remember to review PR #123"
# Creates note with timestamp title, no editor

# Or pipe from stdin
$ echo "Quick thought" | nb new --stdin
```

### Neovim Integration

Add to your Neovim config:

```lua
-- Install the plugin (using lazy.nvim)
{
  dir = "~/path/to/nb-prototype/nvim-plugin",
  config = function()
    require('nb').setup({
      nb_command = "nb",  -- path to nb binary
      mappings = {
        current = '<leader>ni',
        llm = '<leader>nc',
        learn = '<leader>nl',
        new = '<leader>nn',
        quick = '<leader>nq',
        path = '<leader>cp',
        search = '<leader>ns',
        archive = '<leader>na',
      },
    })
  end,
}
```

## Architecture

### No Symlinks

Instead of creating symlinks in every project, workspaces are registered:

```yaml
# ~/.local/share/nb/workspaces.db
- name: backend
  path: /home/user/projects/backend
  type: git-repo
  notebook: /home/user/Documents/nb
```

### Directory Structure

```
~/Documents/nb/
├── repos/
│   └── backend/
│       └── main/
│           ├── current/
│           ├── llm/
│           └── archive/
└── global/
    ├── current/
    └── learn/
```

### Full-Text Search

SQLite FTS5 provides instant search across all notes:

```sql
CREATE VIRTUAL TABLE notes_fts USING fts5(
    path, workspace, branch, type, title, content
);
```

## Development

### Project Structure

```
nb-prototype/
├── cmd/nb/           # CLI commands
├── pkg/
│   ├── workspace/    # Workspace registry
│   ├── service/      # Core service layer
│   └── search/       # Search indexing
└── nvim-plugin/      # Neovim integration
```

### Adding Features

1. **New Commands**: Add to `cmd/nb/`
2. **Core Logic**: Update `pkg/service/`
3. **Neovim Features**: Extend `nvim-plugin/lua/nb/`

## Limitations

This is a prototype demonstrating the core concepts:

- Basic error handling
- Limited git integration
- No web UI yet
- No plugin system
- Basic templates only

## Next Steps

To build a production version:

1. Add comprehensive error handling
2. Implement git hooks
3. Add web server mode
4. Create plugin architecture
5. Add LLM integration
6. Implement performance optimizations
7. Add comprehensive tests