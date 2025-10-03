## v0.3.1-nightly.403ebaa (2025-10-03)

## v0.2.14 (2025-10-01)

This release introduces documentation for `grove-notebook`, covering everything from initial setup and examples to integration with Neovim, Obsidian, and `grove-flow` (6979f17, 460a70b). The documentation generation process itself was improved with support for table of contents generation and other configuration updates (30fe0d0, c47ad2c, 57cd2c3). The release workflow has also been updated to extract release notes directly from the `CHANGELOG.md` file for better consistency (6a4871b).

The interactive note manager (`nb manage`) has been enhanced with improved filtering capabilities, including a new command-line flag for type filtering, an interactive type selection menu, and a more comprehensive search function that filters across multiple fields (61ea4cd). The manager's UI has been migrated to use a centralized theme for better visual consistency (3833216) and now includes a standardized help component with more detailed navigation options (83e186b). A bug in the Neovim plugin that caused misaligned columns in note pickers has also been fixed (526c35d).

### Features

- Add comprehensive project documentation and generation configuration (6979f17)
- Add Table of Contents generation support to documentation (30fe0d0)
- Enhance `nb manage` TUI with improved filtering and search capabilities (61ea4cd)
- Implement standardized help component in note manager TUI (83e186b)
- Migrate note manager UI to a centralized theme for visual consistency (3833216)
- Update release workflow to extract release notes from CHANGELOG.md (6a4871b)
- Make documentation more succinct and improve generation rules (67c3fe2)

### Bug Fixes

- Update CI workflow to use `branches: [ none ]` to disable execution (6f9f140)
- **nvim:** Dynamically calculate column widths in note pickers to fix alignment (526c35d)

### Documentation

- Simplify and restructure the entire documentation suite for clarity (460a70b)
- Update docgen configuration and README templates (c47ad2c)
- Rename `Introduction` sections to `Overview` for consistency (c590949)
- Simplify installation instructions to point to the main Grove guide (4cd159e)

### Chores

- Standardize the key order and settings in `docgen.config.yml` (0b8b376)
- Update `.gitignore` to manage `go.work` files and un-ignore `CLAUDE.md` (e1bf55e)
- Add Grove ecosystem files like `.grove` to `.gitignore` (3aca03a)
- Bump and update project and Grove ecosystem dependencies (dd5c123, d83e349)

### File Changes

```
 .github/workflows/ci.yml                  |   4 +-
 .github/workflows/release.yml             |  10 +-
 .gitignore                                |   8 +
 CHANGELOG.md                              | 253 ++++++++++++
 CLAUDE.md                                 |  30 ++
 README.md                                 | 188 ++-------
 cmd/manage.go                             |  17 +
 docs/01-overview.md                       |  47 +++
 docs/02-examples.md                       | 130 ++++++
 docs/03-neovim-plugin.md                  |  98 +++++
 docs/04-obsidian-integration.md           |  53 +++
 docs/05-grove-flow-integration.md         |  68 ++++
 docs/06-configuration.md                  |  82 ++++
 docs/07-command-reference.md              | 498 +++++++++++++++++++++++
 docs/README.md.tpl                        |   5 +
 docs/docgen.config.yml                    |  55 +++
 docs/docs.rules                           |   1 +
 docs/images/grove-notebook-inkscape.svg   | 654 ++++++++++++++++++++++++++++++
 docs/prompts/01-overview.md               |  43 ++
 docs/prompts/02-examples.md               |  10 +
 docs/prompts/03-neovim-plugin.md          |  30 ++
 docs/prompts/04-obsidian-integration.md   |  19 +
 docs/prompts/05-grove-flow-integration.md |  32 ++
 docs/prompts/06-configuration.md          |  17 +
 docs/prompts/07-command-reference.md      |  28 ++
 go.mod                                    |  31 +-
 go.sum                                    | 124 +-----
 internal/tui/manager/model.go             | 377 ++++++++++++++---
 nvim-plugin/lua/nb/mappings.lua           |  87 +++-
 pkg/docs/docs.json                        | 164 ++++++++
 30 files changed, 2811 insertions(+), 352 deletions(-)
```

## v0.2.13 (2025-09-17)

### Bug Fixes

* **nvim:** dynamically calculate column widths in note pickers

### Documentation

* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.14
* **changelog:** update CHANGELOG.md for v0.2.13

### Chores

* bump dependencies
* update Grove dependencies to latest versions
* add Grove ecosystem files
* update readme

### Features

* enhance nb manage with improved filtering capabilities

## v0.2.13 (2025-09-17)

### Bug Fixes

* **nvim:** dynamically calculate column widths in note pickers

### Documentation

* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.14
* **changelog:** update CHANGELOG.md for v0.2.13

### Chores

* bump dependencies
* update Grove dependencies to latest versions
* add Grove ecosystem files
* update readme

### Features

* enhance nb manage with improved filtering capabilities

## v0.2.13 (2025-09-17)

### Chores

* bump dependencies
* update Grove dependencies to latest versions
* add Grove ecosystem files
* update readme

### Features

* enhance nb manage with improved filtering capabilities

### Bug Fixes

* **nvim:** dynamically calculate column widths in note pickers

### Documentation

* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.14
* **changelog:** update CHANGELOG.md for v0.2.13

## v0.2.13 (2025-09-17)

### Features

* enhance nb manage with improved filtering capabilities

### Bug Fixes

* **nvim:** dynamically calculate column widths in note pickers

### Documentation

* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.14
* **changelog:** update CHANGELOG.md for v0.2.13

### Chores

* bump dependencies
* update Grove dependencies to latest versions
* add Grove ecosystem files
* update readme

## v0.2.13 (2025-09-16)

### Chores

* bump dependencies
* update Grove dependencies to latest versions
* add Grove ecosystem files
* update readme

### Features

* enhance nb manage with improved filtering capabilities

### Bug Fixes

* **nvim:** dynamically calculate column widths in note pickers

### Documentation

* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.14
* **changelog:** update CHANGELOG.md for v0.2.13

## v0.2.13 (2025-09-13)

### Documentation

* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.14
* **changelog:** update CHANGELOG.md for v0.2.13

### Chores

* update Grove dependencies to latest versions
* add Grove ecosystem files
* update readme

### Bug Fixes

* **nvim:** dynamically calculate column widths in note pickers

## v0.2.13 (2025-09-13)

### Chores

* update Grove dependencies to latest versions
* add Grove ecosystem files
* update readme

### Documentation

* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.14
* **changelog:** update CHANGELOG.md for v0.2.13

### Bug Fixes

* **nvim:** dynamically calculate column widths in note pickers

## v0.2.13 (2025-09-12)

### Bug Fixes

* **nvim:** dynamically calculate column widths in note pickers

### Documentation

* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.14
* **changelog:** update CHANGELOG.md for v0.2.13

### Chores

* add Grove ecosystem files
* update readme

## v0.2.13 (2025-09-12)

### Documentation

* **changelog:** update CHANGELOG.md for v0.2.13
* **changelog:** update CHANGELOG.md for v0.2.14
* **changelog:** update CHANGELOG.md for v0.2.13

### Bug Fixes

* **nvim:** dynamically calculate column widths in note pickers

### Chores

* add Grove ecosystem files
* update readme

## v0.2.13 (2025-09-12)

### Chores

* add Grove ecosystem files
* update readme

### Bug Fixes

* **nvim:** dynamically calculate column widths in note pickers

### Documentation

* **changelog:** update CHANGELOG.md for v0.2.14
* **changelog:** update CHANGELOG.md for v0.2.13

## v0.2.14 (2025-08-28)

### Bug Fixes

* **nvim:** dynamically calculate column widths in note pickers

### Chores

* add Grove ecosystem files

## v0.2.13 (2025-08-27)

### Chores

* update readme

## v0.2.12 (2025-08-26)

### Bug Fixes

* add checkout step to release job for gh release create

## v0.2.11 (2025-08-26)

### Bug Fixes

* add back cross compilation!

## v0.2.10 (2025-08-26)

### Bug Fixes

* revert workflows

### Chores

* standardize release asset naming to use BINARY_NAME

## v0.2.9 (2025-08-26)

### Bug Fixes

* binary name
* binary name and cross-compilation in release

## v0.2.8 (2025-08-26)

### Bug Fixes

* disable CGO for cross-compilation in build-all target

## v0.2.7 (2025-08-26)

### Continuous Integration

* enable CGO for SQLite builds in CI workflows

## v0.2.6 (2025-08-25)

### Chores

* bump dependencies

### Continuous Integration

* add Git LFS disable to release workflow
* disable Git LFS and linting in workflow

## v0.2.5 (2025-08-15)

### Bug Fixes

* resolve CI test failures (#1)
* remove GOPROXY=direct

### Continuous Integration

* switch to Linux runners to reduce costs
* consolidate to single test job on macOS
* reduce test matrix to macOS with Go 1.24.4 only

### Chores

* bump deps

### Features

* add interactive note manager with TUI
* add ci.yml

## v0.2.4 (2025-08-12)

### Chores

* bump deps

## v0.2.3 (2025-08-08)

### Features

* **grove-notebook:** Add version injection support

### Chores

* **deps:** bump dependencies
* add grove-core dependency

