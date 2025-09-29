Generate documentation for integrating grove-notebook with grove-flow for storing plans and chats.

## Requirements
Create a section about using grove-notebook as an optional storage backend for grove-flow:

1. **Overview of Integration**
   - How grove-notebook can serve as persistent storage for grove-flow plans and chat sessions
   - Benefits of centralizing AI development artifacts in your notebook
   - The relationship between workspaces and notebook storage

2. **Configuration**
   - Setting grove-flow directories in grove.yml configuration
   - Example configuration from ~/.config/grove/grove.yml:
     ```yaml
     flow:
       oneshot_model: gemini-2.5-pro
       plans_directory: '~/Documents/nb/repos/{{REPO}}/main/plans'
       chat_directory: '~/Documents/nb/repos/{{REPO}}/main/current'
       summarize_on_complete: false
       summary_model: claude-3-haiku
     ```
   - The `{{REPO}}` template variable automatically expands to the current repository name
   - This creates a structured hierarchy: notebook → repos → [repo-name] → [branch] → plans/current
   - How grove-flow automatically uses these directories for storing plans and chat sessions
   - Directory structure created by grove-flow within the notebook

3. **Working with Plans and Chats**
   - How `flow plan` commands store plans in the notebook
   - Chat session storage and organization
   - Searching across plans and chats using `nb search`
   - Viewing plans and chats with both CLI and GUI tools (Obsidian)

