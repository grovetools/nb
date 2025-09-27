# Core Concepts

Understanding the fundamental concepts of `nb` will help you leverage its full potential for organizing and managing your notes.

## The Notebook Directory

The notebook directory is the central location where all your notes are stored. By default, this is located at `~/Documents/nb/`, though it can be customized during workspace registration.

### Directory Structure

```
~/Documents/nb/
├── global/                  # Global workspace for non-project notes
│   ├── current/            # General global notes
│   ├── todos/              # Personal todo lists
│   └── learn/              # Learning notes not tied to projects
└── repos/                   # Project-specific workspaces
    ├── my-project/         # Repository name
    │   ├── main/           # Branch name
    │   │   ├── current/    # Default note type
    │   │   ├── issues/     # Issue tracking
    │   │   │   └── bugs/   # Nested type for bugs
    │   │   └── architecture/
    │   └── feature-branch/  # Another branch
    │       └── current/
    └── another-project/
        └── main/
            └── current/
```

This hierarchical structure ensures that:
- Notes are automatically organized by context (workspace, branch, type)
- Related notes are grouped together
- Navigation remains intuitive even with thousands of notes
- Filesystem operations (backup, sync) work naturally

## Workspaces

A workspace is a registered project directory that `nb` knows about. Workspaces provide context for your notes, determining where they're stored and how they're organized.

### Workspace Types

`nb` supports several workspace types, each optimized for different workflows:

#### 1. Git Repository (`git-repo`)

The most common workspace type, designed for software projects using Git:
- **Automatic branch detection**: Notes are organized by Git branch
- **Branch isolation**: Each branch has its own note collection
- **Cross-branch search**: Find notes from other branches when needed
- **Path structure**: `~/Documents/nb/repos/<repo-name>/<branch>/<note-type>/`

Example:
```bash
cd ~/projects/web-app
nb workspace add .
# Creates: ~/Documents/nb/repos/web-app/main/current/
```

#### 2. Directory (`directory`)

For projects without Git or when you don't need branch-based organization:
- **Simple structure**: No branch subdivision
- **Consistent location**: Notes always go to the same place
- **Path structure**: `~/Documents/nb/<workspace-name>/<note-type>/`

Example:
```bash
cd ~/documents/research
nb workspace add . --type directory
# Creates: ~/Documents/nb/research/current/
```

#### 3. Global (`global`)

A special workspace for notes that transcend individual projects:
- **Always available**: Accessible from any directory
- **Personal notes**: Ideas, todos, learning notes
- **No repository context**: Notes aren't tied to any project
- **Path structure**: `~/Documents/nb/global/<note-type>/`

Example:
```bash
nb new -g "2024 Learning Goals"
# Creates: ~/Documents/nb/global/current/20240115-2024-learning-goals.md
```

#### 4. Monorepo (`monorepo`)

Optimized for large repositories with multiple projects:
- **Performance optimized**: Handles large codebases efficiently
- **Same as git-repo**: Otherwise behaves identically to git-repo type

### Workspace Registry

All workspace information is stored in a SQLite database at `~/.local/share/nb/workspaces.db`. This serves as the source of truth for:
- Registered workspace paths
- Workspace types and names
- Last accessed timestamps
- Custom notebook directories

## Note Types

Note types provide semantic organization for your notes. Each type corresponds to a subdirectory within your workspace, making it easy to categorize and find related content.

### Primary Note Types

`nb` includes several built-in note types, each with its own purpose and optional template:

| Type | Purpose | Default Template |
|------|---------|------------------|
| `current` | General workspace notes (default) | Standard frontmatter |
| `llm` | LLM/AI chat sessions | Includes `started` timestamp |
| `learn` | Learning and study notes | Structured learning template |
| `daily` | Daily journal entries | Date-based with sections |
| `issues` | Bug reports and problems | Issue tracking template |
| `architecture` | Design decisions and docs | ADR-style template |
| `todos` | Task lists and action items | Checklist format |
| `quick` | Rapid capture notes | Minimal frontmatter |
| `blog` | Blog post drafts | Publication metadata |
| `prompts` | Reusable LLM prompts | Usage documentation |

### Nested Types

One of `nb`'s most powerful features is support for nested note types. These create hierarchical categorization through subdirectories:

```bash
# Create a bug report under issues
nb new -t issues/bugs "Login fails on Safari"
# Creates: .../current-workspace/issues/bugs/20240115-login-fails-on-safari.md

# Create an architecture decision record
nb new -t architecture/decisions "Use PostgreSQL for user data"
# Creates: .../current-workspace/architecture/decisions/20240115-use-postgresql.md

# Arbitrary nesting depth is supported
nb new -t todos/sprint/week-3 "Complete API documentation"
# Creates: .../current-workspace/todos/sprint/week-3/20240115-complete-api.md
```

Nested types automatically:
- Generate hierarchical tags from the path
- Create necessary subdirectories
- Maintain consistent naming conventions
- Support type-specific templates (if defined)

## Frontmatter

Every note in `nb` contains YAML frontmatter that stores metadata. This structured data enables powerful search, filtering, and organization capabilities.

### Standard Fields

```yaml
---
id: 20240115-143022-api-design          # Unique identifier
title: API Design Decisions              # Human-readable title
aliases:                                 # Alternative names (optional)
  - REST API Design
  - API Architecture
tags:                                    # Categorization tags
  - api
  - design
  - architecture
repository: my-project                   # Source repository name
branch: main                            # Git branch
created: 2024-01-15T14:30:22-05:00     # Creation timestamp
modified: 2024-01-15T15:45:33-05:00    # Last modification
---
```

### Type-Specific Fields

Different note types may include additional frontmatter fields:

**LLM Notes** (`llm` type):
```yaml
started: 2024-01-15T14:30:22-05:00     # Session start time
model: gpt-4                           # AI model used
```

**Blog Posts** (`blog` type):
```yaml
description: A deep dive into our API design choices
publishDate: 2024-01-20T10:00:00-05:00
updatedDate: 2024-01-22T14:30:00-05:00
draft: true
featured: false
```

### Frontmatter Benefits

The consistent frontmatter structure provides:
- **Unique identification**: Every note has a unique ID
- **Rich metadata**: Timestamps, tags, and context
- **Search optimization**: Indexed for fast full-text search
- **Tool integration**: Parseable by other tools and scripts
- **Migration support**: Standardization via `nb migrate`

## File Naming Conventions

`nb` uses a consistent naming convention for all notes:

```
YYYYMMDD-title-in-kebab-case.md
```

Examples:
- `20240115-api-design-decisions.md`
- `20240115-fix-login-bug.md`
- `20240115-learning-rust-ownership.md`

This convention ensures:
- **Chronological sorting**: Files naturally sort by creation date
- **Readable filenames**: Clear, descriptive names
- **URL-safe**: Compatible with web systems
- **Predictable**: Easy to script and automate

## Context Awareness

`nb` is always aware of your current context, which determines:
- Which workspace is active
- Which Git branch you're on
- Where new notes will be created
- The scope of list and search operations

Check your current context anytime:
```bash
nb context
```

Output:
```
CURRENT CONTEXT:
  Workspace: my-project (git-repo)
  Repository: my-project
  Branch: feature/new-api
  Path: /Users/you/projects/my-project
  Notebook: ~/Documents/nb/repos/my-project/feature-new-api/
```

## Search Index

`nb` maintains a SQLite database with FTS5 (Full-Text Search) capabilities at `~/.local/share/nb/index.db`. This index:

- **Automatically updates**: When notes are created, modified, or deleted
- **Supports advanced queries**: Phrase matching, boolean operators, wildcards
- **Indexes all content**: Title, body, tags, and metadata
- **Provides instant results**: Even with thousands of notes
- **Fallback capability**: Uses LIKE queries if FTS5 isn't available

The search index is crucial for:
```bash
# Fast content search
nb search "authentication AND oauth"

# Cross-workspace discovery
nb search "TODO" --all

# Type-filtered queries
nb search "bug" --type issues
```

## Understanding These Concepts

With these core concepts in mind, you can:
- Organize notes effectively using workspaces and types
- Leverage nested types for detailed categorization
- Use frontmatter for rich metadata
- Take advantage of context awareness for focused work
- Search efficiently across all your knowledge

Next, explore the [Command Reference](05-command-reference.md) to master all of `nb`'s capabilities.