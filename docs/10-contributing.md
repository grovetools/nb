# Contributing Guide

Welcome to the `nb` (grove-notebook) project! We appreciate your interest in contributing. This guide will help you get started with development, understand the codebase, and submit your contributions.

## Getting Started

### Prerequisites

Before you begin development, ensure you have the following installed:

- **Go 1.24.4 or later**: Required for building and testing
  ```bash
  go version  # Should show go1.24.4 or higher
  ```

- **Make**: For running build tasks
  ```bash
  make --version  # GNU Make 3.81 or higher
  ```

- **Git**: For version control
  ```bash
  git --version  # Any recent version
  ```

- **C Compiler**: Required for SQLite CGO support
  - macOS: Xcode Command Line Tools
  - Linux: GCC or Clang

### Setting Up Your Development Environment

1. **Fork and Clone the Repository**
   ```bash
   # Fork on GitHub first, then:
   git clone https://github.com/YOUR_USERNAME/grove-notebook.git
   cd grove-notebook
   ```

2. **Set Up Git Remote**
   ```bash
   git remote add upstream https://github.com/mattsolo1/grove-notebook.git
   git fetch upstream
   ```

3. **Download Dependencies**
   ```bash
   go mod download
   go mod tidy
   ```

4. **Build Development Binary**
   ```bash
   make build
   # Binary will be at ./bin/nb
   ```

5. **Run Tests**
   ```bash
   make test
   # All tests should pass
   ```

## Development Workflow

### Common Development Tasks

The `Makefile` provides all common development tasks:

#### Building

```bash
# Standard build
make build

# Development build with race detector
make dev

# Clean build artifacts
make clean
```

#### Testing

```bash
# Run all tests
make test

# Run tests with verbose output
make test-verbose

# Run tests with coverage
make test-coverage
# Opens coverage.html in browser

# Test specific package
make test-pkg PKG=pkg/service

# Run specific test
make test-run TEST=TestCreateNote
```

#### Benchmarking

```bash
# Run all benchmarks
make bench

# Run specific benchmark
go test -bench=BenchmarkSearch ./pkg/search
```

#### Code Quality

```bash
# Install golangci-lint (first time only)
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Run linter
make lint

# Format code
go fmt ./...

# Run go vet
go vet ./...
```

### Development Cycle

1. **Create a Feature Branch**
   ```bash
   git checkout -b feature/your-feature-name
   ```

2. **Make Your Changes**
   - Write code following the style guide
   - Add tests for new functionality
   - Update documentation if needed

3. **Test Your Changes**
   ```bash
   make test
   make lint
   ```

4. **Commit Your Changes**
   ```bash
   git add .
   git commit -m "feat: add new feature"
   # Use conventional commits (see below)
   ```

5. **Push and Create PR**
   ```bash
   git push origin feature/your-feature-name
   # Create PR on GitHub
   ```

## Project Structure

Understanding the project layout helps navigate the codebase effectively:

### Directory Structure

```
grove-notebook/
├── cmd/                    # CLI command implementations
│   ├── new.go             # nb new command
│   ├── list.go            # nb list command
│   ├── search.go          # nb search command
│   ├── migrate.go         # nb migrate command
│   ├── workspace.go       # nb workspace subcommands
│   └── ...                # Other commands
├── pkg/                    # Core library packages
│   ├── service/           # Business logic orchestration
│   ├── workspace/         # Workspace management
│   ├── search/            # Search index and queries
│   ├── models/            # Data structures
│   ├── frontmatter/       # YAML frontmatter handling
│   ├── migration/         # Note migration logic
│   └── ...                # Other packages
├── internal/              # Internal packages
│   └── tui/              # Terminal UI (Bubble Tea)
├── nvim-plugin/          # Neovim plugin
│   ├── lua/              # Lua plugin code
│   └── README.md         # Plugin documentation
├── obsidian-plugin/      # Obsidian plugin
│   ├── src/              # TypeScript source
│   └── README.md         # Plugin documentation
├── docs/                 # Documentation
├── .github/              # GitHub workflows
│   └── workflows/        # CI/CD pipelines
├── go.mod                # Go module definition
├── go.sum                # Dependency checksums
├── Makefile              # Build automation
└── README.md             # Project overview
```

### Package Responsibilities

#### `cmd/` - Command Layer
- Parses command-line arguments
- Calls service layer functions
- Formats and displays output
- Handles user interaction

#### `pkg/service/` - Service Layer
- Orchestrates business logic
- Coordinates between components
- Manages transactions
- Enforces business rules

#### `pkg/workspace/` - Workspace Management
- Workspace registration and detection
- Git integration
- Context management
- Path resolution

#### `pkg/search/` - Search Infrastructure
- SQLite FTS5 index management
- Query parsing and execution
- Result ranking
- Index maintenance

#### `pkg/models/` - Data Models
- Core data structures
- Type definitions
- Validation logic
- Serialization

#### `pkg/frontmatter/` - Frontmatter Processing
- YAML parsing
- Metadata extraction
- Template rendering
- Validation

#### `internal/tui/` - Terminal UI
- Interactive interface
- Keyboard handling
- State management
- Rendering

## Code Style Guide

### Go Code Style

We follow standard Go conventions with some project-specific guidelines:

#### Naming Conventions

```go
// Package names: lowercase, singular
package workspace

// Exported types: PascalCase
type NoteService struct {}

// Exported functions: PascalCase
func CreateNote() error {}

// Unexported: camelCase
func parseConfig() {}

// Constants: PascalCase or CAPS
const DefaultType = "current"
const MAX_RESULTS = 100

// Interfaces: -er suffix
type Searcher interface {}
```

#### Error Handling

Always wrap errors with context:

```go
// Good
if err := db.Query(sql); err != nil {
    return fmt.Errorf("failed to query database: %w", err)
}

// Bad
if err := db.Query(sql); err != nil {
    return err
}
```

#### Comments

Write clear, concise comments:

```go
// NoteService handles all note-related operations including
// creation, retrieval, and modification of notes.
type NoteService struct {
    // ...
}

// CreateNote creates a new note with the given title and type.
// It returns the path to the created note or an error.
func CreateNote(title string, noteType NoteType) (string, error) {
    // Validate input
    if title == "" {
        return "", errors.New("title cannot be empty")
    }
    
    // ... rest of implementation
}
```

### Commit Message Convention

We use conventional commits for clear history:

```
type(scope): subject

body

footer
```

**Types:**
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `style`: Code style changes (formatting, etc.)
- `refactor`: Code refactoring
- `test`: Test additions or fixes
- `chore`: Build tasks, dependencies, etc.

**Examples:**
```bash
feat(search): add fuzzy search support

fix(migrate): handle special characters in filenames

docs(readme): update installation instructions

refactor(service): extract note creation logic

test(workspace): add tests for git detection
```

## Testing Guidelines

### Test Structure

Place tests alongside the code they test:

```
pkg/
  service/
    service.go
    service_test.go
```

### Writing Tests

```go
func TestCreateNote(t *testing.T) {
    // Arrange
    service := NewService()
    title := "Test Note"
    
    // Act
    path, err := service.CreateNote(title, TypeCurrent)
    
    // Assert
    assert.NoError(t, err)
    assert.Contains(t, path, title)
}
```

### Test Coverage

Aim for at least 80% test coverage:

```bash
make test-coverage
# View coverage.html to identify gaps
```

### Integration Tests

For testing across components:

```go
//go:build integration

func TestFullWorkflow(t *testing.T) {
    // Test complete user workflow
}
```

Run with:
```bash
go test -tags=integration ./...
```

## Pull Request Process

### Before Submitting

1. **Ensure tests pass:**
   ```bash
   make test
   ```

2. **Run linter:**
   ```bash
   make lint
   ```

3. **Update documentation:**
   - Update README if needed
   - Add/update code comments
   - Update relevant docs in `docs/`

4. **Check for breaking changes:**
   - Review API changes
   - Consider backward compatibility
   - Update migration guide if needed

### PR Guidelines

1. **Title**: Use conventional commit format
   ```
   feat(workspace): add monorepo support
   ```

2. **Description**: Include:
   - What changes were made
   - Why they were made
   - How to test them
   - Related issues

3. **Size**: Keep PRs focused and small
   - One feature/fix per PR
   - Break large changes into smaller PRs

4. **Tests**: Include tests for new code
   - Unit tests for functions
   - Integration tests for workflows

5. **Documentation**: Update as needed
   - Code comments
   - README updates
   - API documentation

### PR Template

```markdown
## Description
Brief description of changes

## Type of Change
- [ ] Bug fix
- [ ] New feature
- [ ] Breaking change
- [ ] Documentation update

## Testing
- [ ] Tests pass locally
- [ ] New tests added
- [ ] Manual testing completed

## Checklist
- [ ] Code follows style guide
- [ ] Self-review completed
- [ ] Documentation updated
- [ ] No breaking changes (or documented)
```

## Development Tips

### Debugging

```go
// Add debug logging
log.Printf("DEBUG: workspace=%s, type=%s\n", workspace, noteType)

// Use delve debugger
dlv debug . -- new "Test Note"
```

### Performance Profiling

```bash
# CPU profiling
go test -cpuprofile=cpu.prof -bench=.
go tool pprof cpu.prof

# Memory profiling  
go test -memprofile=mem.prof -bench=.
go tool pprof mem.prof
```

### Local Testing

```bash
# Test with local binary
./bin/nb new "Test Note"

# Test with different config
NB_DATA_DIR=/tmp/nb-test ./bin/nb init
```

## Areas for Contribution

### Good First Issues

Look for issues labeled `good-first-issue`:
- Documentation improvements
- Test coverage increases
- Simple bug fixes
- Code cleanup

### Feature Ideas

- Additional note types
- New search capabilities
- Editor integrations
- Export formats
- Sync mechanisms

### Performance Improvements

- Search optimization
- Startup time reduction
- Memory usage optimization
- Batch operation improvements

## Community

### Getting Help

- **Issues**: [GitHub Issues](https://github.com/mattsolo1/grove-notebook/issues)
- **Discussions**: [GitHub Discussions](https://github.com/mattsolo1/grove-notebook/discussions)
- **Documentation**: This guide and files in `docs/`

### Code of Conduct

- Be respectful and inclusive
- Welcome newcomers
- Give constructive feedback
- Focus on what's best for the community

### Recognition

Contributors are recognized in:
- Git history
- CONTRIBUTORS file
- Release notes

## Release Process

### Version Numbering

We use semantic versioning (MAJOR.MINOR.PATCH):
- MAJOR: Breaking changes
- MINOR: New features
- PATCH: Bug fixes

### Release Checklist

1. Update version in code
2. Update CHANGELOG.md
3. Run full test suite
4. Build release binaries
5. Create GitHub release
6. Update documentation

## Resources

### Helpful Documentation

- [Go Documentation](https://golang.org/doc/)
- [Effective Go](https://golang.org/doc/effective_go.html)
- [SQLite FTS5](https://www.sqlite.org/fts5.html)
- [Bubble Tea TUI Framework](https://github.com/charmbracelet/bubbletea)

### Tools

- [golangci-lint](https://golangci-lint.run/)
- [Delve Debugger](https://github.com/go-delve/delve)
- [Go Report Card](https://goreportcard.com/)

## Thank You!

Your contributions make `nb` better for everyone. Whether it's fixing a typo, adding a feature, or improving performance, every contribution matters.

Happy coding! 🚀