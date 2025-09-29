# Configuration

The `nb` command-line tool can be configured through a YAML file, environment variables, or command-line flags. This document outlines the available configuration options.

## Configuration File

`nb` uses a configuration file located at `$HOME/.config/nb/config.yaml`. This file is automatically created with default values when you first run a command like `nb init`.

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
# This allows you to define the initial content for new notes.
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

The following keys can be set in your `config.yaml` file:

-   **`data_dir`**: Specifies the directory where `nb` stores its operational data. This includes the SQLite databases for the workspace registry (`workspaces.db`) and the search index (`index.db`).
    -   **Default**: `~/.local/share/nb`

-   **`editor`**: The command used to open your text editor when creating or editing notes (e.g., `nb new "My Note"`).
    -   **Default**: The value of the `$EDITOR` environment variable. If `$EDITOR` is not set, it may fall back to system defaults like `vi`.

-   **`default_type`**: The default note category to use when running `nb new` without the `-t` or `--type` flag.
    -   **Default**: `current`

-   **`templates`**: A map allowing you to define custom content templates for different note types. When you create a new note of a specified type, `nb` will use the corresponding template for its initial content.
    -   **Variables**: Templates can contain the following variables, which will be automatically substituted:
        -   `{{.Title}}`: The title of the note.
        -   `{{.Date}}`: The current date (e.g., `2025-09-26`).
        -   `{{.Timestamp}}`: The current timestamp (e.g., `2025-09-26 15:04:05`).
        -   `{{.Workspace}}`: The name of the current workspace.
        -   `{{.Branch}}`: The name of the current Git branch.

## Environment Variables

All settings in the `config.yaml` file can be overridden by environment variables. The variables must be prefixed with `NB_`, be in uppercase, and use underscores instead of hyphens.

-   **`NB_DATA_DIR`**: Overrides the `data_dir` setting.
-   **`NB_EDITOR`**: Overrides the `editor` setting.
-   **`NB_DEFAULT_TYPE`**: Overrides the `default_type` setting.

**Example:**

```bash
export NB_EDITOR="code --wait"
nb new "My Note" # This will open the note in Visual Studio Code.
```