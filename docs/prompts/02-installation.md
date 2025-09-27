# Documentation Task: Installation Guide for nb

You are an expert technical writer. Based on the `Makefile`, `README.md`, and CI workflows (`.github/workflows/`), create a comprehensive installation guide.

**Task:**
1.  **Prerequisites**:
    - List the required tools: Go (specify the version from `ci.yml`), a C compiler (like GCC or Clang) for CGO, and Git.
    - Mention that SQLite with the FTS5 extension is required on the system, but this is standard on most modern OSes.

2.  **Installation Methods**:
    - **From Source (Recommended)**:
        - Provide instructions to clone the repository.
        - Explain how to build the binary using `make build`.
        - Specify that the binary will be located at `./bin/nb`.
        - Instruct the user to move or symlink the binary to a location in their system's PATH (e.g., `/usr/local/bin`).
    - **From GitHub Releases**:
        - Explain that users can download pre-compiled binaries from the GitHub Releases page for their specific OS and architecture (Linux, macOS).
        - Provide instructions on how to download, make the binary executable (`chmod +x`), and move it to their PATH.