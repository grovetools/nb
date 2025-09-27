# Architecture Overview

This document provides a high-level overview of `nb`'s architecture, explaining how the various components work together to provide a seamless note-taking experience.

## System Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                         CLI Layer                           │
│                      cmd/*.go files                         │
│                    (Command handlers)                       │
└─────────────────┬───────────────────────────────────────────┘
                  │
┌─────────────────▼───────────────────────────────────────────┐
│                      Service Layer                          │
│                   pkg/service/service.go                    │
│              (Business logic orchestration)                 │
└──┬──────────┬─────────┬──────────┬─────────┬──────────────┘
   │          │         │          │         │
┌──▼────┐ ┌──▼────┐ ┌──▼────┐ ┌──▼────┐ ┌──▼────┐
│Workspace│ │Search │ │Models │ │Front- │ │Migra- │
│Registry │ │Index  │ │       │ │matter │ │tion   │
└────────┘ └───────┘ └───────┘ └───────┘ └───────┘
     │          │
┌────▼──────────▼─────────────────────────────────────────────┐
│                      Storage Layer                          │
│              SQLite Databases + Filesystem                  │
│          (workspaces.db, index.db, *.md files)             │
└─────────────────────────────────────────────────────────────┘
```

## Core Components

### 1. Service Layer (`pkg/service`)

The service layer is the heart of `nb`, orchestrating all operations and maintaining consistency across components.

**Key Responsibilities:**
- Note CRUD operations (Create, Read, Update, Delete)
- Workspace context management
- Coordination between storage systems
- Template processing
- Business rule enforcement

**Core Types:**
```go
type Service struct {
    registry    *workspace.Registry
    index       *search.Index
    config      *Config
    dataDir     string
    notebookDir string
}
```

**Key Methods:**
- `CreateNote()` - Orchestrates note creation with proper metadata
- `ListNotes()` - Retrieves notes with filtering and sorting
- `SearchNotes()` - Coordinates full-text search across workspaces
- `MoveNote()` - Handles note relocation with index updates
- `ArchiveNotes()` - Manages note archival process

### 2. Workspace Management (`pkg/workspace`)

Manages the registration and detection of workspaces, providing context awareness for all operations.

**Components:**
- `Registry` - Manages workspace database
- `Detector` - Identifies workspace type and context
- `Workspace` - Represents a registered workspace

**Storage:**
- **Database**: `~/.local/share/nb/workspaces.db`
- **Schema**:
  ```sql
  CREATE TABLE workspaces (
      name TEXT PRIMARY KEY,
      path TEXT NOT NULL,
      type TEXT NOT NULL,
      notebook_dir TEXT,
      created_at TIMESTAMP,
      last_accessed TIMESTAMP
  );
  ```

**Workspace Types:**
```go
const (
    TypeGitRepo  = "git-repo"   // Git repository workspace
    TypeDirectory = "directory"  // Simple directory workspace  
    TypeGlobal   = "global"      // Global notes workspace
    TypeMonorepo = "monorepo"    // Large repository optimization
)
```

### 3. Search Index (`pkg/search`)

Provides full-text search capabilities using SQLite's FTS5 extension.

**Architecture:**
- **FTS5 Table**: Optimized full-text search
- **Metadata Table**: Additional note information
- **Fallback**: LIKE queries when FTS5 unavailable

**Database**: `~/.local/share/nb/index.db`

**Schema:**
```sql
-- Full-text search table
CREATE VIRTUAL TABLE notes_fts USING fts5(
    path, 
    workspace, 
    branch, 
    type, 
    title, 
    content,
    tokenize='porter unicode61'
);

-- Metadata table
CREATE TABLE notes_meta (
    path TEXT PRIMARY KEY,
    workspace TEXT,
    branch TEXT,
    type TEXT,
    created_at TIMESTAMP,
    modified_at TIMESTAMP,
    archived BOOLEAN,
    INDEX idx_workspace (workspace),
    INDEX idx_type (type)
);
```

**Search Flow:**
1. Query parsing and validation
2. FTS5 query construction
3. Result retrieval and ranking
4. Metadata enrichment
5. Context filtering

### 4. Data Models (`pkg/models`)

Defines core data structures used throughout the application.

**Note Model:**
```go
type Note struct {
    ID         string
    Title      string
    Path       string
    Type       NoteType
    Tags       []string
    Repository string
    Branch     string
    Created    time.Time
    Modified   time.Time
    Content    string
}
```

**Note Types:**
```go
type NoteType string

const (
    TypeCurrent      NoteType = "current"
    TypeLLM         NoteType = "llm"
    TypeLearn       NoteType = "learn"
    TypeDaily       NoteType = "daily"
    TypeIssues      NoteType = "issues"
    TypeArchitecture NoteType = "architecture"
    TypeTodos       NoteType = "todos"
    TypeQuick       NoteType = "quick"
    TypeBlog        NoteType = "blog"
    TypePrompts     NoteType = "prompts"
)
```

### 5. Frontmatter Processing (`pkg/frontmatter`)

Handles YAML frontmatter parsing and generation for note metadata.

**Core Functions:**
- `Parse()` - Extracts frontmatter from markdown content
- `Build()` - Generates YAML frontmatter from metadata
- `Update()` - Modifies existing frontmatter
- `Validate()` - Ensures frontmatter consistency

**Frontmatter Structure:**
```yaml
id: string                  # Unique identifier
title: string              # Note title
aliases: []string          # Alternative names
tags: []string            # Categorization
repository: string        # Source repository
branch: string           # Git branch
created: time.Time      # Creation timestamp
modified: time.Time     # Last modification
```

## Data Flow Examples

### Note Creation Flow

```
User Input (nb new "Title")
    ↓
CLI Parser (cmd/new.go)
    ↓
Service.CreateNote()
    ├→ Workspace.GetContext() - Determine location
    ├→ Template.Render() - Apply template
    ├→ Frontmatter.Build() - Add metadata
    ├→ Filesystem.Write() - Save file
    ├→ SearchIndex.Add() - Index content
    └→ Editor.Open() - Launch editor
```

### Search Flow

```
User Query (nb search "term")
    ↓
CLI Parser (cmd/search.go)
    ↓
Service.SearchNotes()
    ├→ Query.Parse() - Process search terms
    ├→ Index.Search() - Execute FTS5 query
    ├→ Filter.Apply() - Apply workspace/type filters
    ├→ Results.Enrich() - Add metadata
    └→ Display.Render() - Format output
```

### Migration Flow

```
Migration Request (nb migrate --all)
    ↓
Migration.Analyze()
    ├→ Filesystem.Walk() - Find all notes
    ├→ Frontmatter.Parse() - Check existing metadata
    └→ Issues.Identify() - Find problems
    ↓
Migration.Execute()
    ├→ Backup.Create() - Save .bak files
    ├→ Frontmatter.Fix() - Update metadata
    ├→ Filename.Standardize() - Rename files
    ├→ Tags.Generate() - Create from path
    └→ Index.Rebuild() - Update search
```

## Storage Architecture

### Filesystem Layout

```
~/Documents/nb/                    # Notebook root
├── global/                       # Global workspace
│   ├── current/                 # Note types
│   ├── todos/
│   └── learn/
└── repos/                       # Repository workspaces
    └── project-name/           # Repository name
        ├── main/               # Branch name
        │   ├── current/       # Note types
        │   ├── issues/
        │   │   └── bugs/     # Nested types
        │   └── architecture/
        └── feature-branch/    # Another branch
            └── current/
```

### Database Architecture

**Workspaces Database** (`workspaces.db`):
- Workspace registry
- Configuration storage
- Access tracking

**Search Index** (`index.db`):
- Full-text search index
- Note metadata cache
- Query optimization

### File Format

Standard note structure:
```markdown
---
[YAML Frontmatter]
---

[Markdown Content]
```

Filename convention:
```
YYYYMMDD-kebab-case-title.md
```

## Internal Components

### Terminal UI (`internal/tui`)

Interactive interface built with Bubble Tea framework.

**Architecture:**
- **Model**: Application state and data
- **Update**: Event handling and state mutations
- **View**: Rendering logic

**Key Features:**
- List navigation
- Multi-select operations
- Real-time filtering
- Keyboard shortcuts

### Configuration (`cmd/config`)

Manages application configuration using Viper.

**Hierarchy:**
1. Command flags (highest priority)
2. Environment variables
3. Configuration file
4. Default values (lowest priority)

## Performance Optimizations

### Search Performance

1. **FTS5 Indexing**: Tokenized full-text search
2. **Prepared Statements**: Cached SQL queries
3. **Connection Pooling**: Reused database connections
4. **Lazy Loading**: Content loaded on demand

### Filesystem Operations

1. **Batch Processing**: Group file operations
2. **Parallel Walks**: Concurrent directory traversal
3. **Cache Warming**: Preload frequently accessed data
4. **Incremental Updates**: Only process changes

### Memory Management

1. **Stream Processing**: Large files processed in chunks
2. **Buffer Pools**: Reused memory allocations
3. **Garbage Collection**: Optimized for low latency

## Extension Points

### Plugin System (Future)

Planned architecture for extensibility:

```go
type Plugin interface {
    Name() string
    Initialize(service *Service) error
    OnNoteCreate(note *Note) error
    OnNoteUpdate(note *Note) error
    OnSearch(query string) ([]Note, error)
}
```

### Template System

Custom templates through configuration:
```yaml
templates:
  custom-type: |
    # {{.Title}}
    Custom template content
```

### Hook System

Event-driven extensions:
- Pre-create hooks
- Post-save hooks
- Search filters
- Custom validators

## Security Considerations

### Data Protection

1. **File Permissions**: Respects OS file permissions
2. **Path Validation**: Prevents directory traversal
3. **Input Sanitization**: Protects against injection
4. **Backup Creation**: Preserves data during operations

### Configuration Security

1. **No Secrets in Config**: Uses environment variables
2. **Validated Input**: All user input validated
3. **Safe Defaults**: Secure default configuration

## Scalability

### Design Goals

- **10,000+ notes**: Handles large note collections
- **Sub-second search**: Fast full-text search
- **Minimal memory**: Efficient resource usage
- **Cross-platform**: Works on Linux, macOS

### Bottlenecks and Solutions

1. **Large Repositories**:
   - Solution: Monorepo type with optimizations
   - Incremental indexing
   - Cached metadata

2. **Search Performance**:
   - Solution: FTS5 with proper indexing
   - Query optimization
   - Result pagination

3. **File Operations**:
   - Solution: Batch processing
   - Async I/O where possible
   - Filesystem caching

## Development Patterns

### Error Handling

Consistent error propagation:
```go
if err != nil {
    return fmt.Errorf("operation failed: %w", err)
}
```

### Logging

Structured logging approach:
```go
log.WithFields(log.Fields{
    "workspace": workspace,
    "type": noteType,
}).Info("Creating note")
```

### Testing Strategy

- **Unit tests**: Package-level testing
- **Integration tests**: Cross-component testing
- **E2E tests**: Full workflow validation

## Future Architecture Considerations

### Planned Enhancements

1. **Distributed Storage**: Support for cloud backends
2. **Real-time Sync**: Multi-device synchronization
3. **Collaborative Editing**: Shared workspaces
4. **Advanced Search**: AI-powered semantic search
5. **Plugin Marketplace**: Community extensions

### Modular Design

The architecture is designed to be modular, allowing for:
- Easy component replacement
- Feature toggling
- Progressive enhancement
- Backward compatibility

## Summary

`nb`'s architecture follows clean architecture principles with clear separation of concerns:

1. **CLI Layer**: User interface and command parsing
2. **Service Layer**: Business logic and orchestration
3. **Domain Layer**: Core models and rules
4. **Infrastructure Layer**: Storage and external services

This design ensures maintainability, testability, and extensibility while providing excellent performance for daily note-taking workflows.