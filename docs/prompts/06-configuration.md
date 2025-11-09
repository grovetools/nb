# Documentation Task: Configuration for nb

You are an expert technical writer. Based on `cmd/config/config.go` and `pkg/service/service.go`, document all configuration options for `nb`.

**Task:**
1.  **Configuration File**:
    - Specify the location and format: `$HOME/.config/nb/config.yaml`.
    - Provide an example `config.yaml` showing all default settings.
2.  **Configuration Keys**:
    - Detail each key that can be set in the config file:
        - `data_dir`: Explain its purpose (stores SQLite database for workspace registry). Default: `~/.local/share/nb`.
        - `editor`: The command to open the default text editor. Default: `$EDITOR` environment variable.
        - `default_type`: The default note type for `nb new`. Default: `current`.
        - `templates`: Explain how users can define custom templates for different note types.
3.  **Environment Variables**:
    - Explain that all configuration keys can be overridden by environment variables.
    - Specify the prefix (`NB_`) and format (e.g., `NB_DATA_DIR`).