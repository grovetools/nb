# Grove Flow Integration

`grove-notebook` can serve as a persistent storage backend for `grove-flow`, Grove's LLM job orchestration tool. This integration centralizes development notes, AI-driven plans, and chat sessions into a single, searchable knowledge base. By connecting these tools, you can create a unified repository for all artifacts generated during the software development lifecycle, whether they are created by you or by an AI agent.

This approach treats AI-generated plans and conversations as first-class development artifacts, making them as easy to manage, search, and reference as your own notes.

## Overview of Integration

When `grove-notebook` and `grove-flow` are used together, `grove-flow` stores all its plans and chats within your central notebook directory. 

-   **Unified Storage**: Instead of scattering plan files across different project repositories, they are consolidated into a single, persistent location. This makes it easier to track and manage long-running tasks.
-   **Centralized Knowledge Base**: Your development notes, AI conversations (`flow chat`), and executable workflows (`flow plan`) coexist in one place. This allows you to build a comprehensive knowledge graph of your work, making it simple to find context, review past decisions, and reuse solutions.
-   **Search**: All plans and chats become searchable via `nb search`. You can find specific code snippets, architectural decisions, or task instructions across your history of AI interactions.
-   **Workspace Relationship**: `grove-flow` can be configured to organize plans and chats based on the `nb` workspace they are associated with, maintaining a clear separation of concerns between different projects.

## Configuration

To enable the integration, you configure `grove-flow` to use directories inside your `grove-notebook` storage location. This is typically done in your global Grove configuration file (`~/.config/grove/grove.yml`).

The configuration supports template variables like `{{REPO}}` that automatically expand based on your current context, allowing you to organize plans and chats by repository.

**Example `~/.config/grove/grove.yml` configuration:**

```yaml
flow:
  # Model configuration for grove-flow operations
  oneshot_model: gemini-2.5-pro
  summary_model: claude-3-haiku
  
  # Store plans organized by repository and branch
  # {{REPO}} automatically expands to the current repository name
  plans_directory: '~/Documents/nb/repos/{{REPO}}/main/plans'
  
  # Store active chat sessions in a 'current' directory
  chat_directory: '~/Documents/nb/repos/{{REPO}}/main/current'
  
  # Whether to automatically summarize chats when completed
  summarize_on_complete: false
```

With this setup, `grove-flow` will create a structured hierarchy inside your notebook:

```
~/Documents/nb/
└── repos/
    ├── my-project/
    │   └── main/
    │       ├── plans/
    │       │   ├── feature-auth/
    │       │   │   ├── 01-spec.md
    │       │   │   └── 02-implement.md
    │       │   └── refactor-db/
    │       │       └── ...
    │       └── current/
    │           ├── api-design.md
    │           └── debug-session.md
    └── another-project/
        └── main/
            ├── plans/
            └── current/
```

This organization keeps plans and chats from different repositories separate while maintaining everything within your central notebook.

## Working with Plans and Chats

Once configured, the workflow remains seamless. You use `grove-flow` commands as usual, and the artifacts are automatically stored and organized within your notebook.

-   **Creating Plans**: When you run `flow plan init my-new-feature`, the plan directory is created at `~/Documents/nb/plans/my-new-feature`. All subsequent jobs added with `flow plan add` will be stored there.

-   **Managing Chats**: A command like `flow chat -s my-idea.md` will create the chat file at `~/Documents/nb/chats/my-idea.md`.

-   **Searching Everything**: The main benefit of this integration is the ability to search across all your development artifacts with a single command.
    ```bash
    # Find any mention of "database schema" in notes, plans, or chats
    nb search "database schema" --all-workspaces
    ```

-   **Viewing in Other Tools**: Since all artifacts are just Markdown files in a standard directory structure, you can easily open your notebook in a GUI editor like Obsidian. This allows you to visualize plan dependencies, read through chat logs, and link them to your personal notes.

