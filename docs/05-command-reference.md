# Command Reference

This comprehensive reference covers all `nb` commands, their options, and usage examples.

## Global Options

These options are available on all commands:

| Option | Description |
|--------|-------------|
| `--config <file>` | Specify alternate config file (default: `~/.config/nb/config.yaml`) |
| `--workspace, -W <name>` | Override current workspace context |
| `--help, -h` | Show help for any command |

## nb new

Create a new note and open it in your editor.

### Usage
```bash
nb new [title] [flags]
```

### Flags
| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--type` | `-t` | Note type (e.g., current, learn, issues/bugs) | `current` |
| `--name` | `-n` | Note title/name | - |
| `--no-edit` | - | Don't open editor after creating | `false` |
| `--global` | `-g` | Create note in global workspace | `false` |
| `--stdin` | - | Read content from stdin | Auto-detected |

### Examples
```bash
# Create a note with a title
nb new "API Documentation"

# Create a learning note
nb new -t learn "Rust Ownership Model"

# Create a nested type note
nb new -t issues/bugs "Login fails on mobile"

# Create a global note
nb new -g "Personal reading list"

# Create without opening editor
nb new "Quick reminder" --no-edit

# Pipe content into a new note
echo "Important discovery" | nb new "Research findings"
```

## nb quick

Create a quick note without opening an editor.

### Usage
```bash
nb quick <content>
```

### Examples
```bash
# Capture a quick thought
nb quick "Remember to review the PR before standup"

# Quick notes are timestamped and stored in the 'quick' type
nb quick "Check memory usage on production server"
```

## nb list

List notes with various filtering options.

### Usage
```bash
nb list [type] [flags]
```

### Aliases
- `nb ls`

### Flags
| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--all` | - | List all note types | `false` |
| `--type` | `-t` | Specific note type to list | `current` |
| `--global` | `-g` | List only global notes | `false` |
| `--json` | - | Output in JSON format | `false` |
| `--workspaces` | `-w` | List notes from all workspaces | `false` |
| `--all-branches` | - | List notes from all branches | `false` |

### Examples
```bash
# List current notes in active workspace
nb list

# List all learning notes
nb list learn

# List all note types
nb list --all

# List notes from all workspaces
nb list --workspaces

# List notes from all branches in current repo
nb list --all-branches

# Output as JSON for scripting
nb list --json

# List global todos
nb list todos --global
```

## nb search

Search notes using full-text search.

### Usage
```bash
nb search <query> [flags]
```

### Flags
| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--all` | - | Search all workspaces | `false` |
| `--type` | `-t` | Filter by note type | - |
| `--limit` | - | Maximum results to return | `50` |

### Query Syntax
- **Simple search**: `nb search "API"`
- **Phrase search**: `nb search "\"exact phrase\""`
- **Boolean AND**: `nb search "authentication AND oauth"`
- **Boolean OR**: `nb search "bug OR issue"`
- **Exclusion**: `nb search "API NOT deprecated"`

### Examples
```bash
# Search in current workspace
nb search "authentication"

# Search across all workspaces
nb search "TODO" --all

# Search only in issues
nb search "memory leak" --type issues

# Complex boolean query
nb search "database AND (postgres OR mysql)"

# Limit results
nb search "meeting" --limit 10
```

## nb manage

Open the interactive terminal UI for browsing and managing notes.

### Usage
```bash
nb manage [flags]
```

### Flags
| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--type` | `-t` | Filter notes by type (supports prefix match) | - |

### Keyboard Shortcuts
| Key | Action |
|-----|--------|
| `↑/↓` | Navigate |
| `Space` | Toggle selection |
| `Shift+↑/↓` | Range select |
| `t` | Filter by type |
| `/` | Search within results |
| `s` | Sort by date |
| `x` | Archive selected notes |
| `Enter` | Open in editor |
| `q/Esc` | Quit |

### Examples
```bash
# Open TUI for all notes
nb manage

# Open TUI filtered to issues
nb manage --type issues

# Prefix matching works
nb manage -t iss  # matches "issues"
```

## nb archive

Archive notes to clean up your workspace.

### Usage
```bash
nb archive [files...] [flags]
```

### Flags
| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--older-than` | - | Archive notes older than N days | - |
| `--dry-run` | - | Show what would be archived | `false` |
| `--force` | - | Skip confirmation prompt | `false` |

### Examples
```bash
# Archive specific files
nb archive "20240101-old-meeting.md" "20240102-outdated-spec.md"

# Archive notes older than 30 days
nb archive --older-than 30

# Preview what would be archived
nb archive --older-than 60 --dry-run

# Archive without confirmation
nb archive --older-than 90 --force
```

## nb move

Move or copy notes between types, branches, or workspaces.

### Usage
```bash
nb move <source> <destination> [flags]
```

### Flags
| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--workspace` | - | Target workspace/repository | Current |
| `--branch` | - | Target branch (git repos only) | Current |
| `--type` | - | Target note type | Current |
| `--migrate` | - | Apply migration to standardize | `true` |
| `--dry-run` | - | Preview changes | `false` |
| `--force` | - | Overwrite existing files | `false` |
| `--copy` | - | Copy instead of move | `false` |

### Examples
```bash
# Move note to different type
nb move "api-design.md" --type architecture

# Move to different workspace
nb move "project-notes.md" --workspace other-project

# Move to different branch
nb move "feature-spec.md" --branch develop

# Copy instead of move
nb move "template.md" --workspace new-project --copy

# Move with full path specification
nb move "notes.md" ~/Documents/nb/repos/project/main/docs/

# Preview the operation
nb move "important.md" --type archive --dry-run
```

## nb migrate

Standardize and fix issues in existing notes.

### Usage
```bash
nb migrate [paths...] [flags]
```

### Migration Flags
| Flag | Description |
|------|-------------|
| `--fix-titles` | Extract titles from content if missing |
| `--fix-dates` | Use file mtime if no date in frontmatter |
| `--fix-tags` | Generate tags from path/repo/branch |
| `--fix-ids` | Generate missing IDs |
| `--fix-filenames` | Standardize to YYYYMMDD-title.md format |
| `--index-sqlite` | Create/update SQLite entries |
| `--all` | Apply all fixes |

### Scope Flags
| Flag | Description |
|------|-------------|
| `--global` | Process global notes |
| `--workspace <name>` | Process specific workspace |
| `--all-workspaces` | Process all workspaces |

### Control Flags
| Flag | Description | Default |
|------|-------------|---------|
| `--dry-run` | Preview changes | `false` |
| `--force` | Overwrite existing frontmatter | `false` |
| `--verbose` | Show detailed output | `false` |
| `--report` | Show migration report | `true` |
| `--no-backup` | Don't create .bak files | `false` |

### Examples
```bash
# Apply all fixes to current workspace
nb migrate --all

# Preview changes first
nb migrate --all --dry-run

# Fix only titles and dates
nb migrate --fix-titles --fix-dates

# Migrate specific files
nb migrate notes/*.md --all

# Migrate global notes
nb migrate --global --all

# Migrate all workspaces (use with caution)
nb migrate --all-workspaces --all

# Skip backup creation
nb migrate --all --no-backup
```

## nb workspace

Manage workspace registrations.

### Subcommands

#### workspace add
Register a new workspace.

```bash
nb workspace add [path] [flags]
```

**Flags:**
| Flag | Description | Default |
|------|-------------|---------|
| `--name` | Workspace name | Directory name |
| `--type` | Workspace type (git-repo, directory, global) | `git-repo` |
| `--notebook` | Custom notebook directory | `~/Documents/nb` |

**Examples:**
```bash
# Add current directory
nb workspace add .

# Add with custom name
nb workspace add ~/projects/api --name api-v2

# Add as directory type
nb workspace add ~/documents --type directory
```

#### workspace list
List all registered workspaces.

```bash
nb workspace list
# or
nb workspace ls
```

#### workspace remove
Remove a workspace registration.

```bash
nb workspace remove <name>
# or
nb workspace rm <name>
```

**Examples:**
```bash
# Remove by name
nb workspace remove old-project

# Remove with confirmation
nb workspace rm archived-repo
```

#### workspace current
Show information about the current workspace.

```bash
nb workspace current
```

#### workspace doctor
Check and repair workspace issues.

```bash
nb workspace doctor [flags]
```

**Flags:**
| Flag | Description | Default |
|------|-------------|---------|
| `--fix` | Automatically fix issues | `false` |

**Examples:**
```bash
# Check for issues
nb workspace doctor

# Fix issues automatically
nb workspace doctor --fix
```

## nb context

Display current workspace context information.

### Usage
```bash
nb context [flags]
```

### Flags
| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--json` | - | Output as JSON | `false` |
| `--path` | - | Get specific path (current, llm, learn, etc.) | - |

### Examples
```bash
# Show full context
nb context

# Get as JSON for scripting
nb context --json

# Get specific path
nb context --path current
nb context --path learn
```

## nb init

Initialize nb configuration and directories.

### Usage
```bash
nb init [flags]
```

### Flags
| Flag | Description | Default |
|------|-------------|---------|
| `--minimal` | Only create global workspace | `false` |

### Examples
```bash
# Full initialization
nb init

# Minimal setup
nb init --minimal
```

## nb doctor

System-wide health check and repair tool.

### Usage
```bash
nb doctor [flags]
```

### Flags
| Flag | Description | Default |
|------|-------------|---------|
| `--fix` | Automatically fix issues | `false` |

### Checks Performed
- Configuration file validity
- Database connectivity
- Workspace registrations
- Search index integrity
- Directory permissions

### Examples
```bash
# Run health check
nb doctor

# Auto-fix issues
nb doctor --fix
```

## nb obsidian

Manage Obsidian plugin integration.

### Subcommands

#### obsidian install-dev
Install the Obsidian plugin for development.

```bash
nb obsidian install-dev [flags]
```

**Flags:**
| Flag | Description | Default |
|------|-------------|---------|
| `--vault` | Path to Obsidian vault | `~/Documents/nb` |

**Examples:**
```bash
# Install to default vault
nb obsidian install-dev

# Install to custom vault
nb obsidian install-dev --vault ~/ObsidianVault
```

## nb version

Display version information.

### Usage
```bash
nb version [flags]
```

### Flags
| Flag | Description | Default |
|------|-------------|---------|
| `--json` | Output as JSON | `false` |

### Examples
```bash
# Show version
nb version

# Get version as JSON
nb version --json
```

Output includes:
- Version number
- Git commit hash
- Build date
- Go version
- Platform information

## Command Aliases

Several commands support shorter aliases for convenience:

| Command | Alias |
|---------|-------|
| `nb list` | `nb ls` |
| `nb workspace list` | `nb workspace ls` |
| `nb workspace remove` | `nb workspace rm` |

## Environment Variables

Commands respect these environment variables:

| Variable | Description | Used By |
|----------|-------------|---------|
| `EDITOR` | Default text editor | `nb new` |
| `NB_CONFIG` | Override config file location | All commands |
| `NB_DATA_DIR` | Override data directory | All commands |
| `NO_COLOR` | Disable colored output | All commands |

## Exit Codes

`nb` uses standard exit codes:

| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | General error |
| `2` | Misuse of command |
| `126` | Command cannot execute |
| `127` | Command not found |

## Tips and Tricks

1. **Combine flags for powerful workflows:**
   ```bash
   nb list --all --workspaces --json | jq '.[] | select(.type == "todos")'
   ```

2. **Use type prefixes in manage:**
   ```bash
   nb manage -t iss  # Shows all "issues" notes
   ```

3. **Chain commands with Unix tools:**
   ```bash
   nb search "TODO" --all | grep -E "urgent|critical"
   ```

4. **Script with JSON output:**
   ```bash
   nb context --json | jq -r '.workspace.name'
   ```

5. **Bulk operations with xargs:**
   ```bash
   nb list --json | jq -r '.[] | .path' | xargs -I {} nb archive {}
   ```