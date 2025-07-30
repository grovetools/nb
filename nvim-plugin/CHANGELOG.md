# Changelog

All notable changes to nb.nvim will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## [Unreleased] - 2025-07-08

### Added
- Three new note types with dedicated emoji icons:
  - `issues` (🐛) - Bug reports and problem tracking
  - `architecture` (🏗️) - Design and architecture notes
  - `todos` (✅) - Task lists and project planning
- All notes picker (`<leader>na`)
  - Browse ALL notes across ALL repositories and branches
  - Shows repository/branch information in addition to note type
  - Includes notes from all workspaces, not just current context
  - Supports nested note directories (e.g., issues/bugs, architecture/design)
  - Recursive scanning of all note types
- Enhanced archive command functionality
  - Better file resolution for relative paths
  - Searches entire workspace for matching files
  - Skips archive directory during search
  - Supports archiving by filename without full path
  - Added `--force` flag to skip confirmation prompt (used by Neovim)
  - NbArchive now archives current file by default when no arguments provided
  - Automatically closes buffer after archiving current file
  - Smart confirmation dialog when using archive key mapping
- Service layer improvements
  - New `ListAllNotes()` method for workspace-wide note listing
  - New `ListAllGlobalNotes()` method for global workspace notes
  - Support for nested note type directories
  - Better path resolution for archive operations
- **New `nb migrate` command** for standardizing existing notes:
  - Fixes missing titles by extracting from content, frontmatter, or filename
  - Standardizes filenames to `YYYYMMDD-title.md` format (reduced from `YYYYMMDD-HHMMSS-title.md`)
  - Generates missing IDs, dates, and tags from file metadata
  - Removes redundant repository/branch names from tags
  - Preserves original file timestamps by default (`--preserve-timestamps`)
  - Scope options: current context (default), global, specific workspace, or all
  - Dry-run mode to preview changes before applying
  - Handles filename collisions with automatic numbering
- Improved title extraction in note pickers:
  - Now checks frontmatter `title:` field first
  - Falls back to first `# ` heading in content
  - Finally uses filename pattern extraction
  - Results in fewer "untitled" notes in picker display
- **Enhanced SQLite search indexing system**:
  - Automatic FTS5 detection and graceful fallback to standard SQLite search
  - Eliminates "failed to index note: no such module: fts5" warnings
  - Works with any SQLite installation (with or without FTS5 support)
  - When FTS5 available: advanced full-text search with Porter stemming and relevance ranking
  - When FTS5 unavailable: reliable LIKE-based search on title and content
  - Backward compatible database schema migration for existing installations
- **Complete frontmatter standardization for new notes**:
  - All new notes now include `title:` field in frontmatter (not just heading)
  - Automatic `created:` and `modified:` timestamps on note creation
  - Consistent metadata across all note types (issues, learn, daily, LLM, etc.)
  - Eliminates migration warnings for newly created notes
- Comprehensive test suite for migrate functionality:
  - Tests for title extraction from multiple sources (frontmatter, headings, filenames)
  - Validation of filename standardization and collision handling
  - Multi-line YAML frontmatter parsing support
  - Tag deduplication and cleanup verification
- **Archive notes directly from picker with multi-selection support**:
  - Press `<C-x>` in any note picker to archive selected notes
  - Supports multiple selections: use `<Tab>` to select/unselect notes, then `<C-x>` to archive all selected
  - Use `<C-a>` to select all notes, then `<C-x>` to archive them
  - If no notes are selected, archives the current note under cursor
  - Works in all pickers: repository search (`<leader>ns`), global search (`<leader>ng`), and all notes (`<leader>na`)
  - Shows confirmation dialog with count of notes to be archived
  - Reports success/failure count for batch operations
  - Press `?` in picker to see all available keybindings
  - Fixed selection API to use proper `picker:selected({ fallback = true })` method

### Added (Previous Release) - 2025-07-07
- Global notes search picker (`<leader>ng`)
  - Browse all notes in the global workspace
  - Repository-independent notes access
  - Same rich preview and date display as repository search
- Search picker for browsing all notes in current repository context (`<leader>ns`)
  - Shows notes from all types (current, llm, learn, daily)
  - Displays creation date in YYYY-MM-DD format
  - Shows note type in brackets
  - Live preview of note contents
  - Sorted by date (newest first)
- Table-like display format for note pickers
  - Aligned columns for note type, date, and title
  - Fixed-width formatting for better readability
  - Format: `[type]      YYYY-MM-DD  title`
- Note type selection picker when creating new notes
  - Uses snacks.nvim picker when available
  - Shows emoji icons and descriptions for each type
  - Supports global notes (repository-independent)
  - Customizable width and height
- Support for custom nb binary path configuration
  - Can specify full path to nb executable
  - No longer requires nb to be in PATH
- Global note creation support
  - Notes stored in global workspace
  - Independent of repository/branch context
  - Accessible via `-g` flag or picker option

### Changed
- Note type picker now includes all new note types (issues, architecture, todos)
- List command simplified to use service methods for better consistency
- Archive directory explicitly skipped in all note listing operations
- Updated all note scanners to include new note types
- Archive behavior modified to allow archiving notes with todos if they're old enough
- `<leader>nn` now shows a type selection picker instead of defaulting to "current"
- `<leader>ns` now shows a visual picker with preview instead of text prompt
- Updated utils to use configured nb_command instead of hardcoded "nb"
- Note pickers now sort by modification time instead of creation date
- Note pickers display modification time in "YYYY-MM-DD HH:MM" format
- Picker layout changed to show preview window below list (using custom vertical layout)
- New note filenames now use `YYYYMMDD-title.md` format (simplified from `YYYYMMDD-HHMMSS-title.md`)

### Fixed
- Fixed archive command not working from Neovim due to interactive prompt
- Fixed "Cannot determine current path" error when nb not in PATH
- Fixed picker width/height configuration for better visibility
- Fixed preview window not showing in note search pickers
  - Properly configured file preview using snacks.nvim file format
  - Preview now displays markdown content as you navigate
- **Fixed SQLite FTS5 module warnings**:
  - Eliminated "Warning: failed to index note: no such module: fts5" errors
  - Added automatic FTS5 capability detection with graceful fallback
  - Fixed database schema compatibility issues with existing installations
- **Fixed migrate command frontmatter parsing**:
  - Enhanced YAML parser to handle multi-line arrays (e.g., tags with line breaks)
  - Fixed title extraction from filenames with multiple hyphens
  - Improved tag deduplication to remove redundant repository/branch names
- **Fixed inconsistent note metadata**:
  - New notes now include complete frontmatter preventing migration warnings
  - Fixed missing `title:`, `created:`, and `modified:` fields in new note templates
  - Standardized frontmatter format across all note types

## [0.1.0] - 2025-07-07

### Initial Release
- Basic Neovim integration with nb note system
- Oil.nvim integration for browsing note directories
- Context-aware paths based on current workspace/branch
- Quick note creation commands
- Archive functionality
- Support for multiple note types (current, llm, learn, daily)
- Vim commands for nb operations
- Customizable key mappings
- Fallback to netrw when oil.nvim not available
