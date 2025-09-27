# Documentation Task: Quick Start Guide for nb

You are an expert technical writer. Using the existing `README.md` as a reference, create a slightly more detailed quick-start guide for a new user.

**Task:**
Create a step-by-step tutorial that covers the first 10 minutes of using `nb`.
1.  **Initialization**: Explain the `nb init` command and what it does (creates global configuration in `~/.config/nb` and the notebook directory in `~/Documents/nb`).
2.  **Registering a Workspace**:
    - Show how to navigate to a project directory (`cd ~/projects/my-project`).
    - Explain how to register it using `nb workspace add .`.
3.  **Creating a Note**:
    - Demonstrate creating a new note with a title: `nb new "My First Note"`.
    - Explain that this opens the note in the user's `$EDITOR`.
    - Show how to create a quick, one-line note without an editor: `nb quick "This is a quick thought."`.
4.  **Listing Notes**:
    - Show the basic `nb list` command.
    - Briefly introduce listing notes of a different type, e.g., `nb new -t learn "Go Generics"` followed by `nb list learn`.
5.  **Searching Notes**:
    - Demonstrate a simple search within the current workspace: `nb search "thought"`.
    - Show how to search across all workspaces using the `--all` flag: `nb search "thought" --all`.