# Configuration

The `nb` command-line tool is configured through a YAML file, environment variables, or command-line flags.

## Configuration File

`nb` uses a configuration file located at `$HOME/.config/nb/config.yaml`. The file is created with default values when `nb init` is first run.

**Example `config.yaml`:**

```yaml
# Directory where nb stores its data, including the workspace database and search index.
data_dir: ~/.local/share/nb

# The editor command to use when creating new notes.
# Defaults to the value of the $EDITOR environment variable.
editor: nvim

# The default note type to use for `nb new` if not specified.
default_type: current

# Custom templates for different note types.
templates:
  meeting: |
    # {{.Title}}
    
    **Date:** {{.Date}}
    **Workspace:** {{.Workspace}}
    **Branch:** {{.Branch}}
    
    ## Attendees
    
    - 
    
    ## Agenda
    
    1. 
    
    ## Notes
    
    - 
    
    ## Action Items
    
    - [ ] 
```

## Configuration Keys

The following keys can be set in `config.yaml`:

-   **`data_dir`**: Specifies the directory for operational data, including the SQLite databases for the workspace registry (`workspaces.db`) and the search index (`index.db`).
    -   **Default**: `~/.local/share/nb`

-   **`editor`**: The command used to open a text editor for new or existing notes.
    -   **Default**: The value of the `$EDITOR` environment variable.

-   **`default_type`**: The default note category for the `nb new` command when the `-t` or `--type` flag is not used.
    -   **Default**: `current`

-   **`templates`**: A map that defines initial content for new notes of a specified type.
    -   **Variables**: Templates can contain the following variables, which are substituted upon note creation:
        -   `{{.Title}}`: The title of the note.
        -   `{{.Date}}`: The current date (e.g., `2025-09-26`).
        -   `{{.Timestamp}}`: The current timestamp (e.g., `2025-09-26 15:04:05`).
        -   `{{.Workspace}}`: The name of the current workspace.
        -   `{{.Branch}}`: The name of the current Git branch.

## Environment Variables

Settings in `config.yaml` can be overridden by environment variables. The variables must be prefixed with `NB_`, be in uppercase, and use underscores.

-   **`NB_DATA_DIR`**: Overrides the `data_dir` setting.
-   **`NB_EDITOR`**: Overrides the `editor` setting.
-   **`NB_DEFAULT_TYPE`**: Overrides the `default_type` setting.

**Example:**

```bash
export NB_EDITOR="code --wait"
nb new "My Note"
```