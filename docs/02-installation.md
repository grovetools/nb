# Installation Guide

This guide covers the prerequisites and installation methods for `nb`, the workspace-aware note-taking system.

## Prerequisites

Before installing `nb`, ensure you have the following tools installed on your system:

### Required Tools

- **Go 1.24.4 or later**: Required for building from source
  - Verify with: `go version`
  - Install from: https://golang.org/dl/

- **C Compiler**: Required for CGO support (SQLite compilation)
  - **macOS**: Xcode Command Line Tools (`xcode-select --install`)
  - **Linux**: GCC (`sudo apt-get install build-essential` on Debian/Ubuntu)
  - **Windows**: Not currently supported due to CGO requirements

- **Git**: For version control integration
  - Verify with: `git --version`
  - Install from: https://git-scm.com/downloads

- **SQLite with FTS5**: Full-text search support
  - This is included by default on macOS and most modern Linux distributions
  - The build process will verify FTS5 availability

## Installation Methods

### Method 1: From Source (Recommended)

Building from source ensures you get the latest features and optimizations for your specific platform.

#### Step 1: Clone the Repository

```bash
git clone https://github.com/mattsolo1/grove-notebook.git
cd grove-notebook
```

#### Step 2: Build the Binary

```bash
make build
```

This command will:
- Download all Go dependencies
- Compile the binary with FTS5 support enabled
- Place the executable at `./bin/nb`

#### Step 3: Install the Binary

Option A: Move to system PATH:
```bash
sudo mv ./bin/nb /usr/local/bin/
```

Option B: Create a symbolic link:
```bash
sudo ln -s $(pwd)/bin/nb /usr/local/bin/nb
```

Option C: Add the bin directory to your PATH:
```bash
echo 'export PATH="$PATH:'"$(pwd)/bin"'"' >> ~/.bashrc
source ~/.bashrc
```

#### Step 4: Verify Installation

```bash
nb version
```

You should see version information including the Git commit hash and build date.

### Method 2: From GitHub Releases

Pre-compiled binaries are available for common platforms through GitHub Releases.

#### Step 1: Download the Binary

Visit the [releases page](https://github.com/mattsolo1/grove-notebook/releases) and download the appropriate binary for your platform:

- **macOS Intel**: `nb-darwin-amd64`
- **macOS Apple Silicon**: `nb-darwin-arm64`
- **Linux x64**: `nb-linux-amd64`
- **Linux ARM64**: `nb-linux-arm64`

#### Step 2: Make the Binary Executable

```bash
chmod +x nb-*
```

#### Step 3: Move to System PATH

```bash
# Rename to 'nb' and move to /usr/local/bin
sudo mv nb-* /usr/local/bin/nb
```

#### Step 4: Verify Installation

```bash
nb version
```

## Post-Installation Setup

After installing `nb`, initialize your configuration:

```bash
nb init
```

This command will:
- Create the configuration directory at `~/.config/nb/`
- Initialize the data directory at `~/.local/share/nb/`
- Set up the notebook directory at `~/Documents/nb/`
- Create the global workspace for non-project notes

## Development Installation

For contributors or those who want the latest development features:

### Install with Race Detector

```bash
make dev
```

This builds `nb` with Go's race detector enabled, useful for debugging concurrency issues.

### Run Tests

```bash
make test
```

### Install Linter

```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
make lint
```

## Troubleshooting

### FTS5 Not Available

If you encounter SQLite FTS5 errors:

1. **macOS**: Update to the latest macOS version or install SQLite via Homebrew:
   ```bash
   brew install sqlite3
   ```

2. **Linux**: Install SQLite development packages:
   ```bash
   # Debian/Ubuntu
   sudo apt-get install libsqlite3-dev
   
   # Fedora/RHEL
   sudo dnf install sqlite-devel
   ```

### CGO Compilation Errors

Ensure you have a C compiler installed:
```bash
# Check for compiler
gcc --version
```

If missing, install the appropriate development tools for your platform as described in the Prerequisites section.

### Permission Denied

If you get permission errors when moving the binary:
```bash
# Use sudo for system directories
sudo mv ./bin/nb /usr/local/bin/

# Or use a user-writable location
mkdir -p ~/.local/bin
mv ./bin/nb ~/.local/bin/
echo 'export PATH="$PATH:$HOME/.local/bin"' >> ~/.bashrc
source ~/.bashrc
```

## Uninstallation

To completely remove `nb`:

```bash
# Remove the binary
sudo rm /usr/local/bin/nb

# Remove configuration and data (optional)
rm -rf ~/.config/nb
rm -rf ~/.local/share/nb
rm -rf ~/Documents/nb
```

## Next Steps

Once installed, proceed to the [Quick Start Guide](03-quick-start.md) to begin using `nb` for your note-taking needs.