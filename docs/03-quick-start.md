# Quick Start Guide

This guide will walk you through your first 10 minutes with `nb`, covering initialization, workspace setup, note creation, and search.

## Step 1: Initialize nb

Start by initializing `nb` to set up your configuration and notebook directory:

```bash
nb init
```

This command performs several important setup tasks:
- Creates the global configuration file at `~/.config/nb/config.yaml`
- Initializes the data directory at `~/.local/share/nb/` for SQLite databases
- Sets up your notebook directory at `~/Documents/nb/`
- Creates the `global` workspace for notes that aren't tied to specific projects

You should see output confirming the successful initialization:
```
✓ Created configuration directory: ~/.config/nb
✓ Created data directory: ~/.local/share/nb
✓ Created notebook directory: ~/Documents/nb
✓ Initialized global workspace
```

## Step 2: Register Your First Workspace

Workspaces connect your projects to `nb`. Navigate to one of your project directories and register it:

```bash
cd ~/projects/my-project
nb workspace add .
```

`nb` will automatically detect that this is a Git repository (if applicable) and register it accordingly. You'll see confirmation:
```
✓ Added workspace: my-project (type: git-repo)
```

To verify your workspace was added:
```bash
nb workspace list
```

Output:
```
WORKSPACES:
  my-project (git-repo) - /Users/you/projects/my-project
  global (global) - ~/Documents/nb/global
```

## Step 3: Create Your First Note

Now let's create a note in your newly registered workspace:

```bash
nb new "API Design Decisions"
```

This command will:
1. Create a new note with the title "API Design Decisions"
2. Add YAML frontmatter with metadata (ID, title, timestamps, repository, branch)
3. Open the note in your default editor (`$EDITOR`)

In your editor, you'll see something like:
```markdown
---
id: 20240115-143022-api-design-decisions
title: API Design Decisions
tags: []
repository: my-project
branch: main
created: 2024-01-15T14:30:22-05:00
modified: 2024-01-15T14:30:22-05:00
---

# API Design Decisions

```

Add your content below the frontmatter, save, and exit your editor.

### Quick Notes Without an Editor

For rapid capture of thoughts, use the `quick` command:

```bash
nb quick "Remember to review the authentication flow before the meeting"
```

This creates a timestamped note without opening an editor, perfect for capturing fleeting thoughts.

## Step 4: Create Notes of Different Types

`nb` organizes notes by type. Let's create a learning note:

```bash
nb new -t learn "Understanding Go Generics"
```

This creates a note in the `learn` subdirectory of your workspace. Common note types include:
- `current` - General workspace notes (default)
- `learn` - Learning and study notes
- `issues` - Bug reports and problems
- `todos` - Task lists
- `architecture` - Design decisions

You can even use nested types:
```bash
nb new -t issues/bugs "Login button not responding on mobile"
```

## Step 5: List Your Notes

View all notes in your current workspace:

```bash
nb list
```

Output:
```
NOTES IN my-project (branch: main):
  2024-01-15  API Design Decisions
  2024-01-15  Remember to review the authentication flow...
```

List notes of a specific type:

```bash
nb list learn
```

Or list notes across all your workspaces:

```bash
nb list --workspaces
```

## Step 6: Search Your Notes

Search for notes containing specific terms in the current workspace:

```bash
nb search "authentication"
```

The search uses SQLite's full-text search for fast, relevant results:
```
SEARCH RESULTS:
  my-project/main  2024-01-15  current  Remember to review the authentication flow...
```

Search across all workspaces:

```bash
nb search "API" --all
```

Filter search by note type:

```bash
nb search "bug" --type issues
```

## Step 7: Interactive Note Management

Launch the interactive TUI to browse and manage notes visually:

```bash
nb manage
```

In the TUI, you can:
- Navigate with arrow keys
- Select multiple notes with `space`
- Filter by type with `t`
- Search with `/`
- Archive selected notes with `x`
- Open notes in your editor with `Enter`
- Quit with `q` or `Esc`

## Working with Git Branches

One of `nb`'s most powerful features is Git branch awareness. When you switch branches, your notes follow:

```bash
# Create a note on the main branch
git checkout main
nb new "Main branch architecture notes"

# Switch to a feature branch
git checkout feature/new-api
nb new "API implementation details"

# List notes shows only current branch notes
nb list

# But you can search across all branches
nb search "architecture" --all-branches
```

## Global Notes

For notes that aren't tied to any specific project, use the global workspace:

```bash
nb new -g "Personal learning goals for 2024"
```

Global notes are stored in `~/Documents/nb/global/` and are always accessible regardless of your current directory.

## Next Steps

You're now ready to use `nb` for your daily note-taking! Here are some areas to explore next:

1. **Learn about note types and organization** - See [Core Concepts](04-core-concepts.md)
2. **Master all available commands** - Check the [Command Reference](05-command-reference.md)
3. **Set up editor integration** - Configure [Neovim or Obsidian plugins](07-editor-integrations.md)
4. **Customize your setup** - Explore [Configuration options](06-configuration.md)
5. **Migrate existing notes** - Use the [Migration Guide](08-migration-guide.md)

## Pro Tips

1. **Use `nb context` to see where you are**:
   ```bash
   nb context
   ```

2. **Archive old notes to keep lists clean**:
   ```bash
   nb archive --older-than 30
   ```

3. **Move notes between workspaces or types**:
   ```bash
   nb move "API Design Decisions" --type architecture
   ```

4. **Check system health with doctor**:
   ```bash
   nb doctor
   ```

Happy note-taking with `nb`!