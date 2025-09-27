# Documentation Task: Contributing Guide for nb

You are an expert technical writer. Create a guide for developers who want to contribute to the `nb` project. Use the `Makefile` and `CLAUDE.md` as references.

**Task:**
1.  **Getting Started**:
    - List prerequisites (Go, Make).
    - Explain how to clone the repository and download dependencies (`go mod download`).
2.  **Development Workflow**:
    - Refer to the `Makefile` for common development tasks.
    - `make build`: How to build a development binary.
    - `make dev`: How to build with the race detector.
    - `make test`: How to run unit tests.
    - `make lint`: How to run the linter (mentioning the dependency on `golangci-lint`).
3.  **Project Structure**:
    - Briefly explain the purpose of the main directories:
        - `cmd/`: CLI command definitions.
        - `pkg/`: Core library code.
        - `internal/`: Internal packages not intended for external use (like the TUI).
        - `nvim-plugin/`: Neovim plugin source.
4.  **Pull Requests**: Provide basic guidelines for submitting PRs (e.g., "Ensure tests pass," "Update documentation if necessary").