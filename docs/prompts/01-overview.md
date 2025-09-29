Generate an overview section for grove-notebook.

## Requirements
Create a comprehensive overview that includes:

1. **High-level description**: What grove-notebook is - a command-line note-taking system for developers
2. **Animated GIF placeholder**: Include `<!-- placeholder for animated gif -->`
3. **Central Knowledge Base Concept**: 
   - Emphasize the power of setting the notebook directory to a location outside Git repositories
   - This creates a central, persistent knowledge base (similar to a Zettelkasten)
   - Notes persist across projects and branches, building a personal development knowledge graph
4. **Key features**: 
   - Markdown-based notes with full-text search
   - Optional storage backend for grove-flow plans and AI chat sessions
   - Neovim plugin with grove-flow integrations (execute chats, add plans/jobs)
   - Obsidian compatibility for visualization and rich plugin ecosystem
   - Simple CLI for quick note creation and retrieval
5. **Ecosystem Integration**: 
   - Can serve as unified storage for grove-flow plans and chats
   - Neovim plugin enables executing AI chats directly on markdown files
   - Compatible with Obsidian for enhanced visualization (note: plugin requires source installation)
6. **Installation**: Include brief installation instructions at the bottom

## Installation Format
Include this condensed installation section at the bottom:

### Installation

Install via the Grove meta-CLI:
```bash
grove install notebook
```

Verify installation:
```bash
nb version
```

Requires the `grove` meta-CLI. See the [Grove Installation Guide](https://github.com/mattsolo1/grove-meta/blob/main/docs/02-installation.md) if you don't have it installed.

## Context
Grove-notebook provides a command-line, workspace-aware note-taking system for developers that integrates with Git and provides powerful search capabilities.
