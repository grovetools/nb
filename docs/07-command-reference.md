# Command Reference

This document provides a reference for all `nb` command-line interface commands, organized by function.

---

### `nb new`

Creates a new note and opens it in the default editor.

**Usage**

```bash
nb new [title] [flags]
```

**Description**

This command creates a new note file with standardized frontmatter. The filename is generated based on the current date and the provided title. By default, it opens the new note in the editor defined by the `$EDITOR` environment variable. Content can also be piped directly into the note via stdin.

**Arguments & Flags**

| Flag        | Shorthand | Description                                                                                                                                                             | Default   |
| ----------- | --------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------- |
| `[title]`   | (Arg)     | An optional title for the note. If omitted, a title will be generated from the timestamp.                                                                               | (none)    |
| `--type`    | `-t`      | The type of note to create. Supports nested types like `issues/bugs`. Common types: `current`, `llm`, `learn`, `daily`, `issues`, `architecture`, `todos`, `blog`, `prompts`. | `current` |
| `--name`    | `-n`      | An alternative way to specify the note's title.                                                                                                                         | (none)    |
| `--no-edit` |           | Prevents the command from opening an editor after the note is created.                                                                                                  | `false`   |
| `--global`  | `-g`      | Creates the note in the global workspace, making it independent of any project or repository.                                                                           | `false`   |
| `--stdin`   |           | Reads the note's content from standard input. This is auto-detected when content is piped.                                                                              | `false`   |

**Examples**

```bash
# Create a new note with a title, which opens in your editor
nb new "API design session notes"

# Create a new 'learn' note about Go generics
nb new -t learn "Go Generics"

# Create a global daily note without opening an editor
nb new -g -t daily --no-edit

# Pipe content directly into a new note
echo "This is an important idea." | nb new "A Quick Thought"
```

---

### `nb quick`

Creates a quick, one-line note without opening an editor.

**Usage**

```bash
nb quick "<content>"
```

**Description**

This command is a shortcut for creating a note of type `quick`. It takes the note's content as a single argument, generates a timestamped title, and saves the file without opening an editor. It is for capturing thoughts or reminders from the command line.

**Arguments & Flags**

| Flag        | Shorthand | Description                                 | Default |
| ----------- | --------- | ------------------------------------------- | ------- |
| `[content]` | (Arg)     | The content of the quick note (required).   | (none)  |

**Examples**

```bash
# Create a quick reminder
nb quick "Remember to review the new pull request #42."
```

---

### `nb list`

Lists notes, defaulting to the current workspace context.

**Usage**

```bash
nb list [type] [flags]
```

**Aliases**: `ls`

**Description**

Displays a table of notes. By default, it shows notes of type `current` in the active workspace and branch. Various flags allow you to expand the scope to other note types, branches, and workspaces.

**Arguments & Flags**

| Flag             | Shorthand | Description                                                               | Default   |
| ---------------- | --------- | ------------------------------------------------------------------------- | --------- |
| `[type]`         | (Arg)     | An optional note type to filter the list by (e.g., `llm`, `issues`).      | `current` |
| `--all`          |           | List notes of all types within the current context.                       | `false`   |
| `--type`         | `-t`      | An alternative way to specify the note type to filter by.                 | `current` |
| `--global`       | `-g`      | List notes from the global workspace only.                                | `false`   |
| `--workspaces`   | `-w`      | List notes from all registered workspaces.                                | `false`   |
| `--all-branches` |           | List all notes from all branches within the current Git repository.       | `false`   |
| `--json`         |           | Output the list of notes in JSON format.                                  | `false`   |

**Examples**

```bash
# List all notes of type 'current' in the active workspace and branch
nb list

# List all 'llm' notes in the current context
nb list llm

# List all notes of all types in the current context
nb list --all

# List all notes across all workspaces and branches as JSON
nb list --workspaces --json
```

---

### `nb search`

Performs a full-text search across notes.

**Usage**

```bash
nb search <query> [flags]
```

**Description**

Searches the content and titles of notes using ripgrep (or grep as a fallback) for full-text queries. The search is scoped to the current workspace by default.

**Arguments & Flags**

| Flag      | Shorthand | Description                                      | Default |
| --------- | --------- | ------------------------------------------------ | ------- |
| `<query>` | (Arg)     | The search query (required).                     | (none)  |
| `--all`   |           | Search across all registered workspaces.         | `false` |
| `--type`  | `-t`      | Filter search results by a specific note type.   | (none)  |
| `--limit` |           | The maximum number of search results to return.  | `50`    |

**Examples**

```bash
# Search for "API authentication" in the current workspace
nb search "API authentication"

# Search for "database" in 'learn' notes across all workspaces
nb search "database" --all -t learn
```

---

### `nb archive`

Moves one or more notes to the archive directory.

**Usage**

```bash
nb archive [files...] [flags]
```

**Description**

Moves specified notes into a structured archive directory within the current workspace context. It can also archive notes based on their age.

**Arguments & Flags**

| Flag           | Shorthand | Description                                                               | Default |
| -------------- | --------- | ------------------------------------------------------------------------- | ------- |
| `[files...]`   | (Arg)     | A space-separated list of note filenames to archive.                      | (none)  |
| `--older-than` |           | Archive all notes older than the specified number of days.                | `0`     |
| `--dry-run`    |           | Show which notes would be archived without actually moving them.          | `false` |
| `--force`      |           | Archive notes without a confirmation prompt.                              | `false` |

**Examples**

```bash
# Archive two specific notes
nb archive 20250101-old-note.md 20250215-another.md

# Archive all notes in the current workspace older than 90 days
nb archive --older-than 90
```

---

### `nb move`

Moves or copies notes between different types, branches, or workspaces.

**Usage**

```bash
nb move <file> <destination> [flags]
```

**Description**

Reorganizes notes by moving or copying them. The source is a file path, and the destination can be a note type (like `learn`), a full directory path, or a combination of workspace, branch, and type flags. By default, it also applies the `migrate` logic to standardize the note in its new location.

**Arguments & Flags**

| Flag          | Shorthand | Description                                                                                             | Default |
| ------------- | --------- | ------------------------------------------------------------------------------------------------------- | ------- |
| `<file>`      | (Arg)     | The source note file to move (required).                                                                | (none)  |
| `<dest>`      | (Arg)     | The destination note type or path. Required if not using flags.                                         | (none)  |
| `--workspace` |           | The name of the target workspace.                                                                       | (current) |
| `--branch`    |           | The name of the target Git branch.                                                                      | (current) |
| `--type`      |           | The target note type.                                                                                   | (none)  |
| `--migrate`   |           | Apply standardization (frontmatter, filename) to the note after moving.                                 | `true`  |
| `--dry-run`   |           | Preview the move operation without making changes.                                                      | `false` |
| `--force`     |           | Overwrite the destination file if it already exists.                                                    | `false` |
| `--copy`      |           | Copy the note instead of moving it, leaving the original file intact.                                   | `false` |

**Examples**

```bash
# Move a note from 'current' to 'learn' within the same workspace
nb move my-note.md learn

# Move a note to a different workspace and branch
nb move my-note.md --workspace other-project --branch feature-branch --type current

# Copy an external file into the 'llm' notes of the current workspace
nb move /path/to/external.md llm --copy
```

---

### `nb migrate`

Standardizes the frontmatter and filenames of existing notes.

**Usage**

```bash
nb migrate [paths...] [flags]
```

**Description**

Analyzes notes and fixes common formatting and metadata issues, which is useful for importing notes from other systems or cleaning up an existing collection. It can fix missing titles, dates, IDs, and tags, and standardize filenames.

**Arguments & Flags**

| Flag                 | Shorthand | Description                                                        | Default    |
| -------------------- | --------- | ------------------------------------------------------------------ | ---------- |
| `[paths...]`         | (Arg)     | Optional list of specific files or directories to migrate.         | (current)  |
| `--all`              |           | Apply all available fixes.                                         | `false`    |
| `--fix-titles`       |           | Add missing `title` fields to frontmatter.                         | `false`    |
| `--fix-dates`        |           | Add missing `created` and `modified` timestamps.                   | `false`    |
| `--fix-tags`         |           | Add tags based on the note's directory structure.                  | `false`    |
| `--fix-ids`          |           | Add a unique ID if one is missing.                                 | `false`    |
| `--fix-filenames`    |           | Rename files to the `YYYYMMDD-title.md` standard.                  | `false`    |
| `--dry-run`          |           | Preview all changes without modifying any files.                   | `false`    |
| `--workspace`        |           | Limit the migration to a specific workspace by name.               | (current)  |
| `--global`           |           | Run the migration on the global workspace only.                    | `false`    |
| `--all-workspaces`   |           | Run the migration on all registered workspaces.                    | `false`    |
| `--no-backup`        |           | Do not create backup files.                                        | `false`    |
| `--preserve-timestamps` |        | Preserve original file modification times.                         | `true`     |

**Example**

```bash
# Preview all fixes for all notes in the current workspace
nb migrate --all --dry-run
```

---

### `nb workspace`

Manages workspace registrations.

**Usage**

```bash
nb workspace <subcommand>
```

**Description**

A collection of commands to manage how `nb` recognizes and interacts with project directories.

**Subcommands**

-   **`add [path]`**: Registers a directory as a workspace. If `[path]` is omitted, the current directory is used.
    -   `--name`: Specify a custom name for the workspace.
    -   `--type`: Set the workspace type (`git-repo`, `directory`, `global`).
    -   `--notebook`: Set a custom notebook directory for this workspace.

-   **`list`** (alias: `ls`): Displays a table of all registered workspaces, their paths, and last-used timestamps.

-   **`remove <name>`** (alias: `rm`): Removes a workspace registration by its name.

-   **`current`**: Shows detailed information about the currently detected workspace.

-   **`doctor`**: (See `nb doctor` command below).

**Examples**

```bash
# Register the current directory as a workspace
nb workspace add

# List all registered workspaces
nb workspace list

# Remove a workspace registration
nb workspace remove my-old-project
```

---

### `nb doctor`

Checks for and offers to fix common configuration issues.

**Usage**

```bash
nb doctor [flags]
```

**Description**

Scans the workspace registration database for problems like duplicate entries, invalid paths, or inconsistent path casing and provides an option to fix them automatically.

**Arguments & Flags**

| Flag    | Shorthand | Description                              | Default |
| ------- | --------- | ---------------------------------------- | ------- |
| `--fix` |           | Automatically fix any detected issues.   | `false` |

**Example**

```bash
# Check the configuration for issues
nb doctor

# Check for and automatically fix issues
nb doctor --fix
```

---

### `nb context`

Displays information about the current workspace context.

**Usage**

```bash
nb context [flags]
```

**Description**

Provides details about the detected workspace, active Git branch, and the absolute paths for different note types. This is primarily used for scripting and integration with other tools.

**Arguments & Flags**

| Flag     | Shorthand | Description                                         | Default |
| -------- | --------- | --------------------------------------------------- | ------- |
| `--json` |           | Output the context information in JSON format.      | `false` |
| `--path` |           | Return only the absolute path for a specific type.  | (none)  |

**Example**

```bash
# Get the absolute path to the 'learn' notes directory for the current context
nb context --path learn
```

---

### `nb init`

Initializes `nb` configuration and registers the current directory as a workspace.

**Usage**

```bash
nb init [flags]
```

**Description**

This command sets up the necessary global configuration and directories for `nb` (e.g., in `~/.config/nb` and `~/.local/share/nb`). It also registers the current directory as your first workspace.

**Arguments & Flags**

| Flag        | Shorthand | Description                                                           | Default |
| ----------- | --------- | --------------------------------------------------------------------- | ------- |
| `--minimal` |           | Only create the global configuration and directories, do not register the current directory. | `false`   |

**Example**

```bash
# Navigate to your project and initialize nb
cd ~/projects/my-project
nb init
```

---

### `nb obsidian`

Manages Obsidian integration.

**Usage**

```bash
nb obsidian <subcommand>
```

**Description**

A collection of commands for integrating `nb` with the Obsidian note-taking application.

**Subcommands**

-   **`install-dev`**: Installs the `nb` Obsidian plugin for development purposes by creating a symlink from the plugin source code to an Obsidian vault's plugin directory. This is intended for developers working on the plugin itself.
    -   `--vault`: Specify the path to the Obsidian vault. Defaults to `~/Documents/nb`.

**Example**

```bash
# Install the plugin for development in a custom vault
nb obsidian install-dev --vault /path/to/my/obsidian/vault
```

---

### `nb version`

Prints the version information for the binary.

**Usage**

```bash
nb version [flags]
```

**Description**

Displays the version, commit hash, branch, and build date for the `nb` binary.

**Arguments & Flags**

| Flag     | Shorthand | Description                                 | Default |
| -------- | --------- | ------------------------------------------- | ------- |
| `--json` |           | Output the version information in JSON format. | `false`   |

**Example**

```bash
# Get version information as JSON
nb version --json
```