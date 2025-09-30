<!-- DOCGEN:OVERVIEW:START -->

<img src="docs/images/grove-notebook-inkscape.svg" width="60%" />

`grove-notebook` (`nb`) is a command-line note-taking system designed for the Grove Ecosystem. It organizes notes around your project workspaces and Git branches, providing a fast, searchable, and version-controlled way to manage development knowledge. It integrates with tools like Neovim, Obsidian, and the broader Grove ecosystem.

<!-- placeholder for animated gif -->

## The Central Knowledge Base

A core concept of `grove-notebook` is the creation of a central, persistent knowledge base. By default, the main notebook directory (`~/Documents/nb`) is stored outside of any specific Git repository. This design allows your notes to function like a [Zettelkasten](https://en.wikipedia.org/wiki/Zettelkasten), creating a personal development knowledge graph that grows over time.

## Key Features

*   **Markdown-Based Notes**: All notes are plain Markdown files, making them portable, versionable, and easy to edit with any tool.
*   **`grove-flow` Storage Backend**: Acts as the central storage system for `grove-flow` plans and AI chat sessions, unifying your development notes and AI-assisted workflows in one place.
*   **Neovim Plugin**: A Neovim plugin allows you to create notes, search your knowledge base, and execute `grove-flow` commands (like running chats or adding jobs to a plan) directly from the editor.
*   **Obsidian Compatibility**: The notebook directory can be opened as an Obsidian vault, giving you a graphical interface for visualizing, linking, and working with your notes using Obsidian's extensive plugin ecosystem.
*   **Simple CLI**: A clean and efficient command-line interface makes it fast to create, find, and manage notes without leaving the terminal.

## Installation

Install via the Grove meta-CLI:
```bash
grove install nb
```

Verify installation:
```bash
nb version
```

Requires the `grove` meta-CLI. See the [Grove Installation Guide](https://github.com/mattsolo1/grove-meta/blob/main/docs/02-installation.md) if you don't have it installed.

<!-- DOCGEN:OVERVIEW:END -->


<!-- DOCGEN:TOC:START -->

See the [documentation](docs/) for detailed usage instructions:
- [Overview](docs/01-overview.md) - <img src="./images/grove-notebook-inkscape.svg" width="60%" />
- [Examples](docs/02-examples.md) - This document provides a series of practical examples to demonstrate how to u...
- [Neovim Plugin](docs/03-neovim-plugin.md) - The `nb.nvim` plugin provides a cohesive experience for managing `grove-noteb...
- [Obsidian Integration](docs/04-obsidian-integration.md) - `grove-notebook` works effectively with GUI-based Markdown editors like Obsid...
- [Grove Flow Integration](docs/05-grove-flow-integration.md) - `grove-notebook` can serve as a persistent storage backend for `grove-flow`, ...
- [Configuration](docs/06-configuration.md) - The `nb` command-line tool can be configured through a YAML file, environment...
- [Command Reference](docs/07-command-reference.md) - This document provides a comprehensive reference for all `nb` command-line in...

<!-- DOCGEN:TOC:END -->
