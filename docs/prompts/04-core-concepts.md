# Documentation Task: Core Concepts of nb

You are an expert technical writer. Based on `pkg/workspace/`, `pkg/models/`, and `pkg/service/`, explain the fundamental concepts of `nb`.

**Task:**
1.  **The Notebook Directory**:
    - Explain the default location (`~/Documents/nb`) and its structure.
    - Describe the subdirectories: `global` for global notes and `repos` for workspace-specific notes.
2.  **Workspaces**:
    - Define what a workspace is (a registered project directory).
    - Explain the different types, referencing `pkg/workspace/workspace.go`:
        - `git-repo`: The most common type. Explain that notes are scoped to branches.
        - `directory`: For non-Git projects.
        - `global`: A special workspace for notes not tied to any project.
    - Reference the `workspaces.db` file in the data directory as the source of truth.
3.  **Note Types**:
    - Explain that notes are categorized by type, which corresponds to a subdirectory.
    - List the primary note types defined in `pkg/models/note.go`: `current`, `llm`, `learn`, `daily`, `issues`, `architecture`, `todos`, `quick`, `blog`, `prompts`.
    - Crucially, explain that **nested types** are supported (e.g., `issues/bugs`, `architecture/decisions`), which create corresponding subdirectories.
4.  **Frontmatter**:
    - Explain that every note contains YAML frontmatter for metadata.
    - Based on `pkg/frontmatter/frontmatter.go`, list the key fields: `id`, `title`, `tags`, `repository`, `branch`, `created`, `modified`.