# Documentation Task: Architecture Overview of nb

You are an expert technical writer. Provide a high-level overview of the `nb` architecture based on the structure of the `pkg/` directory.

**Task:**
1.  **Core Components**: Describe the role of each major package:
    - `pkg/service`: The central service layer that orchestrates all operations.
    - `pkg/workspace`: Manages the registration and detection of workspaces. Explain that it uses a SQLite database (`workspaces.db`) to store this information.
    - `pkg/search`: Manages the full-text search index. Explain that it uses another SQLite database (`index.db`) and supports FTS5 for advanced searching.
    - `pkg/models`: Defines the core data structures like `Note` and `NoteType`.
    - `pkg/frontmatter`: The utility for parsing and building YAML frontmatter in note files.
2.  **Data Flow**: Briefly explain the data flow for a common command like `nb new`.
    - CLI (`cmd/new.go`) parses arguments.
    - `service.CreateNote` is called.
    - The service uses the `workspace.Registry` to find the current context.
    - It constructs the file path and generates content with frontmatter.
    - The file is written to disk.
    - The `search.Index` is updated with the new note's metadata and content.