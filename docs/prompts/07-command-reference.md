# Documentation Task: CLI Command Reference for nb

You are an expert technical writer. Create a comprehensive reference for all `nb` CLI commands. Analyze every file in the `cmd/` directory to build this reference.

**Task:**
For each of the following commands, provide:
- A brief description of what it does.
- The usage syntax.
- A detailed explanation of all its arguments, flags, and options.
- At least one practical example.

**Commands to Document:**
- `nb new` (from `cmd/new.go`)
- `nb quick` (from `cmd/quick.go`)
- `nb list` (from `cmd/list.go`)
- `nb search` (from `cmd/search.go`)
- `nb archive` (from `cmd/archive.go`)
- `nb move` (from `cmd/move.go`)
- `nb migrate` (from `cmd/migrate.go`)
- `nb workspace` (include subcommands `add`, `list`, `remove`, `current` from `cmd/workspace.go`)
- `nb doctor` (from `cmd/doctor.go` and `cmd/workspace.go`)
- `nb context` (from `cmd/context.go`)
- `nb init` (from `cmd/init.go`)
- `nb obsidian` (include subcommand `install-dev` from `cmd/obsidian.go`)
- `nb version` (from `cmd/version.go`)

Structure the output with clear headings for each command. This should be the most detailed and extensive document.