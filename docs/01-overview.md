# Grove-Notebook: Workspace-Aware Note-Taking System

The `nb` command, provided by the `grove-notebook` repository, is a command-line, workspace-aware note-taking system designed specifically for developers who live in the terminal. It seamlessly integrates with your existing development workflow, organizing notes within the context of your software projects while maintaining full Git branch awareness.

## Core Philosophy: Context-Driven Note Organization

Grove-Notebook is built on the principle that notes should automatically organize themselves around your development context. Instead of forcing developers to maintain separate note-taking systems, it integrates directly into their workflow, ensuring that thoughts and documentation naturally align with the code and projects they're working on.

The tool serves developers who value speed, efficiency, and keyboard-driven interfaces while maintaining the sophisticated organization needed for complex software projects.

## Dual Role: Note Organizer and Workflow Enhancer

The `nb` system serves as both an intelligent note organizer that understands your project structure and a workflow enhancer that bridges the gap between quick thoughts and structured documentation.

## Key Features

### Workspace-Aware Organization
`nb` understands your project structure and automatically organizes notes accordingly:
- **Git repositories**: Notes are scoped to specific branches, keeping feature work isolated
- **Directory workspaces**: Simple folder-based organization for non-Git projects
- **Global workspace**: A central location for notes that transcend individual projects

### Git Branch Integration
When working in a Git repository, `nb` automatically:
- Detects your current branch and organizes notes accordingly
- Maintains separate note collections for each branch
- Enables cross-branch search when you need to find information from other contexts
- Preserves the connection between your notes and the code they document

### Full-Text Search with SQLite FTS5
Powered by SQLite's FTS5 (Full-Text Search) extension, `nb` provides:
- Instant search across all your notes
- Sophisticated query capabilities including phrase matching and boolean operators
- Context-aware filtering by workspace, branch, or note type
- Lightning-fast performance even with thousands of notes

### Comprehensive CLI
Every aspect of note management is accessible through intuitive commands:
- Create notes with `nb new` or capture quick thoughts with `nb quick`
- List and browse notes with `nb list`
- Search across workspaces with `nb search`
- Manage notes interactively with the TUI via `nb manage`
- Archive, move, and migrate notes with dedicated commands

### Interactive TUI for Note Management
The `nb manage` command launches a powerful terminal user interface featuring:
- Visual note browsing with keyboard navigation
- Multi-select capabilities for bulk operations
- Real-time filtering and search
- Direct editing and archiving from the interface
- Context-aware display showing workspace and branch information

### Seamless Editor Integrations
Native integrations bring `nb` directly into your editor:
- **Neovim plugin**: Create, search, and manage notes without leaving your editor
- **Obsidian plugin**: Bridge the gap between terminal and GUI note-taking
- Custom key mappings for rapid note creation and search
- Live preview and intelligent filtering

### Data Integrity and Migration Tools
Maintain consistency and reliability with built-in tools:
- `nb doctor` diagnoses and repairs configuration issues
- `nb migrate` standardizes existing notes to the `nb` format
- Automatic frontmatter generation and validation
- Filename normalization to maintain consistent organization
- Backup creation during migration operations

## Target Audience

`nb` is built for:
- **Developers** who spend their day in the terminal and want their notes to follow their workflow
- **Technical writers** documenting software projects who need context-aware organization
- **DevOps engineers** managing multiple environments and configurations
- **Anyone using Git** who wants their notes to align with their version control workflow
- **Power users** who value speed, efficiency, and keyboard-driven interfaces

Whether you're debugging a complex issue, planning a new feature, or capturing meeting notes, `nb` ensures your thoughts are captured in context and remain discoverable when you need them most.