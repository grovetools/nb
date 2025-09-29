# Integrating with Obsidian

`grove-notebook` works effectively with GUI-based Markdown editors like Obsidian. Obsidian provides a rich environment for visualizing, organizing, and interacting with your notes. Using them together creates a complementary system that leverages the strengths of both tools.

## Using Your Notebook as an Obsidian Vault

The core of the integration is straightforward: you can open your `nb` notebook directory as an Obsidian vault. By default, your notes are stored in `~/Documents/nb`, which you can select when using Obsidian's "Open folder as vault" option.

### Benefits of Combined Usage

-   **Best of Both Worlds**: Combine the speed and scriptability of the `nb` command line with the rich user interface, linking capabilities, and visualization tools of Obsidian.
-   **Single Source of Truth**: All your notes, whether created via the CLI, Neovim, or within Obsidian, reside in the same file system, ensuring a unified and consistent knowledge base.
-   **Seamless Syncing**: Changes are reflected instantly. When you create a note using `nb new`, Obsidian's file watcher will automatically detect and display it. Similarly, edits made in Obsidian are immediately available to `nb search`.
-   **Enhanced Visualization**: Use Obsidian's graph view to discover relationships between notes created across different projects and branches, helping you build a connected knowledge graph.

## Common Workflow Integration

A typical workflow might look like this:

1.  **Quick Capture (CLI)**: While deep in a coding session, you have an idea or need to jot down a note. Instead of switching contexts, you quickly run a command:
    ```bash
    nb quick "Refactor the authentication middleware to use a new strategy."
    ```
2.  **Review and Organize (Obsidian)**: Later, you open your `nb` vault in Obsidian. The new note is already there. You can now:
    -   Link it to other relevant notes, such as the project's architecture document or previous meeting notes.
    -   Add it to a Kanban board using an Obsidian plugin.
    -   Refine and expand the note using Obsidian's rich text editor.
    -   Use plugins like Dataview to create dynamic queries and summaries of your notes.

This workflow allows you to capture information with minimal disruption via the CLI and handle deeper organization within a dedicated visual tool.

## Obsidian Plugin (WIP/Experimental)

An experimental Obsidian plugin is available within the `grove-notebook` repository to provide more direct integration. It adds a dedicated sidebar view that allows you to browse your `nb` workspaces and notes from within the Obsidian interface.

### Installation

**Note**: The plugin is currently intended for development and testing. It must be installed manually from the source code and is not available in the Obsidian community plugin store.

1.  **Clone the Repository**: Clone the `grove-notebook` repository to your local machine.
    ```bash
    git clone https://github.com/mattsolo1/grove-notebook.git
    ```

2.  **Symlink the Plugin**: Create a symbolic link from the `obsidian-plugin` directory in the repository to your Obsidian vault's plugin directory.
    ```bash
    # Example command
    ln -s /path/to/grove-notebook/obsidian-plugin /path/to/your/vault/.obsidian/plugins/nb-integration
    ```

3.  **Build the Plugin**: Navigate to the plugin's source directory and build it.
    ```bash
    cd /path/to/grove-notebook/obsidian-plugin
    npm install
    npm run dev
    ```

4.  **Enable in Obsidian**: Open Obsidian, go to `Settings` > `Community Plugins`, and enable the "NB Integration" plugin. You may need to refresh the plugin list first.
