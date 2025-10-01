# Integrating with Obsidian

`grove-notebook` stores notes as Markdown files in a standard directory structure, which is compatible with GUI-based editors like Obsidian. Using both tools allows for command-line note creation and graphical organization.

## Using the Notebook Directory as an Obsidian Vault

The `nb` notebook directory can be opened as an Obsidian vault. By default, this directory is `~/Documents/nb`.

-   **File System Synchronization**: Notes created via the `nb` command line are detected by Obsidian's file watcher. Notes created or modified in Obsidian are indexed and available to `nb search`.
-   **Visualization**: Obsidian's graph view can be used to visualize links between notes from different projects and branches.

## Common Workflow

This workflow separates note creation from organization. The CLI is used for capture during development tasks, and Obsidian is used for subsequent review and linking.

1.  **Capture via CLI**: During a coding task, a note is created from the terminal.
    ```bash
    nb quick "Refactor the authentication middleware to use a new strategy."
    ```

2.  **Organize in Obsidian**: Later, the `nb` vault is opened in Obsidian. The new note is present and can be:
    -   Linked to other notes, such as an architecture document.
    -   Managed on a Kanban board via an Obsidian plugin.
    -   Refined using Obsidian's editor.
    -   Queried using plugins like Dataview.

## Obsidian Plugin

An experimental Obsidian plugin is available in the `grove-notebook` source repository. It adds a sidebar view to browse `nb` workspaces and notes.

The plugin is intended for development and testing. It is not in the Obsidian community plugin store and must be installed from source.

### Installation

1.  **Clone the Repository**:
    ```bash
    git clone https://github.com/mattsolo1/grove-notebook.git
    ```

2.  **Symlink the Plugin**: Create a symbolic link from the `obsidian-plugin` directory to your Obsidian vault's plugin directory.
    ```bash
    # Example command
    ln -s /path/to/grove-notebook/obsidian-plugin /path/to/your/vault/.obsidian/plugins/nb-integration
    ```

3.  **Build the Plugin**:
    ```bash
    cd /path/to/grove-notebook/obsidian-plugin
    npm install
    npm run dev
    ```

4.  **Enable in Obsidian**: Open Obsidian, go to `Settings` > `Community Plugins`, refresh the list, and enable the "NB Integration" plugin.