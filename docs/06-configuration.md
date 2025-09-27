# Configuration

`nb` uses a flexible configuration system that supports both file-based configuration and environment variables. This guide covers all configuration options and how to customize `nb` for your workflow.

## Configuration File

The main configuration file is located at `$HOME/.config/nb/config.yaml`. This file is created automatically when you run `nb init` and uses YAML format for readability.

### Default Configuration

Here's the default configuration file with all available settings:

```yaml
# nb configuration file
# Location: ~/.config/nb/config.yaml

# Data directory for SQLite databases
# Stores workspaces.db and index.db
data_dir: ~/.local/share/nb

# Default editor command
# Falls back to $EDITOR environment variable if not set
editor: 

# Default note type for 'nb new' command
# Can be any valid note type (current, learn, issues, etc.)
default_type: current

# Custom templates for different note types
# Use Go template syntax with available variables
templates:
  # Daily note template with date
  daily: |
    # Daily Note - {{.Date}}
    
    ## Tasks
    - [ ] 
    
    ## Notes
    
    ## Tomorrow
    
  # Meeting note template
  meeting: |
    # {{.Title}}
    
    **Date:** {{.Timestamp}}
    **Attendees:** 
    
    ## Agenda
    
    ## Notes
    
    ## Action Items
    
  # Learning note template
  learn: |
    # {{.Title}}
    
    ## Objective
    What I want to learn:
    
    ## Resources
    - 
    
    ## Notes
    
    ## Summary
    Key takeaways:
    
  # Issue tracking template
  issues: |
    # {{.Title}}
    
    ## Description
    
    ## Steps to Reproduce
    1. 
    
    ## Expected Behavior
    
    ## Actual Behavior
    
    ## Environment
    - OS: 
    - Version: 
    
  # Architecture decision record
  architecture: |
    # {{.Title}}
    
    ## Status
    Proposed
    
    ## Context
    
    ## Decision
    
    ## Consequences
    
    ## Alternatives Considered
```

## Configuration Keys

### data_dir

**Type:** String  
**Default:** `~/.local/share/nb`  
**Environment Variable:** `NB_DATA_DIR`

Specifies the directory where `nb` stores its SQLite databases:
- `workspaces.db` - Workspace registry and metadata
- `index.db` - Full-text search index

Example:
```yaml
data_dir: /opt/nb/data
```

### editor

**Type:** String  
**Default:** Value of `$EDITOR` environment variable  
**Environment Variable:** `NB_EDITOR`

Defines the command used to open notes for editing. If not set in the config file, `nb` uses the `$EDITOR` environment variable.

Examples:
```yaml
# Use vim
editor: vim

# Use VS Code
editor: code

# Use Neovim with specific options
editor: nvim -c "set spell"
```

### default_type

**Type:** String  
**Default:** `current`  
**Environment Variable:** `NB_DEFAULT_TYPE`

Sets the default note type used when creating new notes without specifying the `-t` flag.

Examples:
```yaml
# Default to learning notes
default_type: learn

# Default to todo lists
default_type: todos

# Default to architecture notes
default_type: architecture
```

### templates

**Type:** Map of strings  
**Default:** Empty  
**Environment Variable:** Not applicable

Defines custom templates for different note types. Templates use Go's text/template syntax with these available variables:

| Variable | Description | Example |
|----------|-------------|---------|
| `{{.Title}}` | Note title | "API Design" |
| `{{.Date}}` | Current date | "2024-01-15" |
| `{{.Timestamp}}` | Full timestamp | "2024-01-15T14:30:22-05:00" |
| `{{.Type}}` | Note type | "learn" |
| `{{.Repository}}` | Current repository | "my-project" |
| `{{.Branch}}` | Current Git branch | "main" |

Example template configuration:
```yaml
templates:
  # Bug report template
  bugs: |
    # Bug: {{.Title}}
    
    **Reported:** {{.Date}}
    **Severity:** 
    **Status:** Open
    
    ## Description
    
    ## Steps to Reproduce
    
    ## Expected vs Actual
    
    ## Workaround
    
  # Sprint planning template
  sprint: |
    # Sprint Planning - {{.Date}}
    
    ## Sprint Goal
    
    ## Capacity
    - 
    
    ## User Stories
    
    ### Story 1
    - [ ] Task 1
    - [ ] Task 2
    
    ## Risks
    
  # Code review template
  review: |
    # Code Review: {{.Title}}
    
    **PR/MR:** #
    **Author:** 
    **Date:** {{.Timestamp}}
    
    ## Summary
    
    ## Issues Found
    
    ## Suggestions
    
    ## Approval Status
    - [ ] Approved
    - [ ] Needs Changes
```

## Environment Variables

All configuration keys can be overridden using environment variables. This is useful for:
- Temporary overrides
- CI/CD environments
- Docker containers
- Multiple configurations

### Variable Format

Environment variables follow the pattern `NB_<KEY>` where `<KEY>` is the uppercase configuration key:

| Config Key | Environment Variable | Example |
|------------|---------------------|---------|
| `data_dir` | `NB_DATA_DIR` | `NB_DATA_DIR=/tmp/nb` |
| `editor` | `NB_EDITOR` | `NB_EDITOR=nano` |
| `default_type` | `NB_DEFAULT_TYPE` | `NB_DEFAULT_TYPE=todos` |

### Priority Order

Configuration values are resolved in this order (highest to lowest priority):
1. Command-line flags (e.g., `--type`)
2. Environment variables (e.g., `NB_EDITOR`)
3. Configuration file (`~/.config/nb/config.yaml`)
4. Built-in defaults

### Examples

```bash
# Override editor for a single command
NB_EDITOR=emacs nb new "Emacs note"

# Set data directory for a session
export NB_DATA_DIR=/tmp/nb-test
nb init

# Use different config file
nb --config ~/custom-nb.yaml list
```

## Advanced Configuration

### Multiple Configurations

You can maintain multiple configuration files for different contexts:

```bash
# Work configuration
nb --config ~/.config/nb/work.yaml new "Work note"

# Personal configuration  
nb --config ~/.config/nb/personal.yaml new "Personal note"
```

### Shell Aliases

Create shell aliases for common configurations:

```bash
# In ~/.bashrc or ~/.zshrc
alias nb-work='nb --config ~/.config/nb/work.yaml'
alias nb-personal='nb --config ~/.config/nb/personal.yaml'
alias nb-quick='nb new -t quick --no-edit'
```

### Workspace-Specific Settings

While `nb` doesn't support per-workspace configuration files directly, you can achieve similar results with shell functions:

```bash
# Function to set workspace-specific environment
nb-project() {
  case "$1" in
    api)
      NB_DEFAULT_TYPE=architecture nb "$@"
      ;;
    blog)
      NB_DEFAULT_TYPE=blog nb "$@"
      ;;
    *)
      nb "$@"
      ;;
  esac
}
```

## Template Variables Reference

When creating custom templates, you can use these variables:

### Standard Variables

| Variable | Type | Description |
|----------|------|-------------|
| `{{.Title}}` | string | Note title from command line |
| `{{.Date}}` | string | Current date (YYYY-MM-DD) |
| `{{.Timestamp}}` | string | ISO 8601 timestamp |
| `{{.Type}}` | string | Note type being created |
| `{{.Repository}}` | string | Git repository name |
| `{{.Branch}}` | string | Current Git branch |
| `{{.Workspace}}` | string | Workspace name |

### Template Functions

Go template functions available in templates:

| Function | Description | Example |
|----------|-------------|---------|
| `lower` | Convert to lowercase | `{{.Title | lower}}` |
| `upper` | Convert to uppercase | `{{.Type | upper}}` |
| `title` | Title case | `{{.Repository | title}}` |
| `replace` | Replace string | `{{.Branch | replace "/" "-"}}` |

### Conditional Logic

Templates support conditional logic:

```yaml
templates:
  issues: |
    # {{.Title}}
    
    {{if eq .Branch "main"}}
    **⚠️ Production Issue**
    {{else}}
    **Branch:** {{.Branch}}
    {{end}}
    
    ## Description
```

## Validation

`nb` validates configuration on startup. Common validation checks:

1. **Data directory exists and is writable**
   ```bash
   nb doctor  # Checks configuration validity
   ```

2. **Editor command is available**
   ```bash
   nb doctor --fix  # Attempts to fix issues
   ```

3. **Template syntax is valid**
   - Invalid templates are ignored with a warning
   - Use `nb doctor` to check template validity

## Migration from Older Versions

If upgrading from an older version of `nb`:

1. **Backup existing configuration:**
   ```bash
   cp ~/.config/nb/config.yaml ~/.config/nb/config.yaml.backup
   ```

2. **Re-initialize to get new defaults:**
   ```bash
   nb init --minimal
   ```

3. **Merge custom settings:**
   - Compare backup with new file
   - Copy over custom templates and settings

## Troubleshooting

### Configuration Not Loading

Check file location and permissions:
```bash
ls -la ~/.config/nb/config.yaml
```

Validate YAML syntax:
```bash
cat ~/.config/nb/config.yaml | python -m yaml
```

### Environment Variables Not Working

Verify variable is exported:
```bash
echo $NB_EDITOR
export NB_EDITOR=vim  # Make sure to export
```

Check precedence:
```bash
nb context --json | jq .config
```

### Templates Not Applying

Test template directly:
```bash
nb new -t <type> "Test" --dry-run
```

Check for syntax errors:
```bash
nb doctor
```

## Best Practices

1. **Version control your configuration:**
   ```bash
   cd ~/.config
   git init
   git add nb/config.yaml
   git commit -m "Initial nb configuration"
   ```

2. **Document custom templates:**
   ```yaml
   templates:
     # Template for tracking customer issues
     # Used by support team
     customer-issue: |
       ...
   ```

3. **Use environment variables for sensitive data:**
   ```bash
   # Don't put tokens in config.yaml
   export NB_API_TOKEN=secret
   ```

4. **Test configuration changes:**
   ```bash
   nb --config test.yaml doctor
   ```

5. **Keep templates focused:**
   - Create specific templates for specific workflows
   - Don't try to make one template do everything
   - Use type hierarchy (issues/bugs, issues/features)