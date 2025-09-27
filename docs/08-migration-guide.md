# Migration Guide

The `nb migrate` command is a powerful tool for importing existing Markdown notes into the `nb` ecosystem and standardizing notes to ensure consistency. This guide covers common migration scenarios, available options, and best practices.

## Why Use nb migrate?

The migration tool solves several common problems when working with notes from various sources:

### Problems It Solves

1. **Missing Metadata**: Notes without YAML frontmatter lack searchable metadata
2. **Inconsistent Naming**: Files with various naming conventions are hard to organize
3. **No IDs**: Without unique identifiers, notes can't be reliably referenced
4. **Lost Timestamps**: File creation dates don't reflect actual content creation
5. **Poor Organization**: Notes scattered without proper categorization
6. **Search Limitations**: Notes not indexed for full-text search

### What It Does

`nb migrate` performs these operations:

- **Adds YAML Frontmatter**: Creates structured metadata for every note
- **Standardizes Filenames**: Converts to `YYYYMMDD-title.md` format
- **Generates Unique IDs**: Assigns persistent identifiers
- **Preserves Timestamps**: Maintains original creation/modification times
- **Creates Tags**: Generates tags from directory structure
- **Indexes Content**: Updates SQLite search index for instant search

## Common Migration Scenarios

### Scenario 1: Importing Notes from Another System

You have a collection of Markdown notes from Obsidian, Notion, or another tool:

```bash
# 1. Copy notes to your workspace
cp -r ~/ObsidianVault/Notes/* ~/Documents/nb/repos/my-project/main/current/

# 2. Preview what will change
cd ~/projects/my-project
nb migrate --all --dry-run

# 3. Apply all fixes
nb migrate --all

# 4. Verify the migration
nb list
```

### Scenario 2: Standardizing Existing nb Notes

Your existing `nb` notes need updating to the latest format:

```bash
# Check current workspace for issues
nb migrate --dry-run

# Apply specific fixes
nb migrate --fix-ids --fix-filenames

# Or apply everything
nb migrate --all
```

### Scenario 3: Bulk Import from Multiple Sources

Importing notes from various projects into `nb`:

```bash
# Import personal notes to global
cp ~/Desktop/personal-notes/*.md ~/Documents/nb/global/current/
nb migrate --global --all

# Import project docs
cp ~/old-project/docs/*.md ~/Documents/nb/repos/old-project/main/architecture/
nb migrate --workspace old-project --all

# Import everything at once (use carefully)
nb migrate --all-workspaces --all
```

### Scenario 4: Organizing Scattered Notes

You have notes in various formats and locations:

```bash
# Create appropriate type directories
mkdir -p ~/Documents/nb/repos/my-project/main/{issues,architecture,todos}

# Move notes to appropriate types
mv ~/scattered/bugs*.md ~/Documents/nb/repos/my-project/main/issues/
mv ~/scattered/design*.md ~/Documents/nb/repos/my-project/main/architecture/
mv ~/scattered/task*.md ~/Documents/nb/repos/my-project/main/todos/

# Migrate each type with appropriate tags
nb migrate ~/Documents/nb/repos/my-project/main/issues --all
nb migrate ~/Documents/nb/repos/my-project/main/architecture --all
nb migrate ~/Documents/nb/repos/my-project/main/todos --all
```

## Migration Options

### Fix Operations

| Flag | Purpose | Example Impact |
|------|---------|----------------|
| `--fix-titles` | Extract title from first heading or filename | `untitled.md` → Title: "Project Overview" |
| `--fix-dates` | Use file modification time if missing | Adds `created: 2024-01-15T10:30:00Z` |
| `--fix-tags` | Generate from path structure | `/issues/bugs/` → tags: [issues, bugs] |
| `--fix-ids` | Create unique identifiers | Adds `id: 20240115-103022-project-overview` |
| `--fix-filenames` | Standardize to YYYYMMDD format | `My Note.md` → `20240115-my-note.md` |
| `--index-sqlite` | Update search index | Makes content searchable |
| `--all` | Apply all fixes | Complete standardization |

### Scope Control

| Flag | Scope | Use Case |
|------|-------|----------|
| `--global` | Global workspace only | Personal notes |
| `--workspace <name>` | Specific workspace | Single project migration |
| `--all-workspaces` | Every workspace | Full system standardization |
| (default) | Current workspace | Day-to-day maintenance |

### Control Flags

| Flag | Purpose | Default |
|------|---------|---------|
| `--dry-run` | Preview without changes | false |
| `--force` | Overwrite existing frontmatter | false |
| `--verbose` | Show detailed progress | false |
| `--report` | Display summary report | true |
| `--no-backup` | Skip .bak file creation | false |

## Step-by-Step Migration Workflow

### Step 1: Prepare Your Notes

1. **Gather all notes** in a temporary directory:
   ```bash
   mkdir ~/temp-notes
   cp -r ~/various/sources/*.md ~/temp-notes/
   ```

2. **Review and organize** by type:
   ```bash
   # Create type directories
   mkdir ~/temp-notes/{issues,docs,learning,todos}
   
   # Manually sort or use patterns
   mv ~/temp-notes/*bug*.md ~/temp-notes/issues/
   mv ~/temp-notes/*learn*.md ~/temp-notes/learning/
   ```

### Step 2: Copy to nb Structure

```bash
# Copy to appropriate workspace and type
cp -r ~/temp-notes/issues/* ~/Documents/nb/repos/my-project/main/issues/
cp -r ~/temp-notes/learning/* ~/Documents/nb/global/learn/
```

### Step 3: Preview Changes

Always preview before applying:

```bash
# See what will change
nb migrate --all --dry-run > migration-preview.txt

# Review the preview
less migration-preview.txt
```

### Step 4: Apply Migration

```bash
# Run migration with all fixes
nb migrate --all

# Output shows:
# ✓ Fixed frontmatter: 45 files
# ✓ Renamed files: 23 files  
# ✓ Generated IDs: 45 files
# ✓ Indexed in SQLite: 45 files
# ✓ Created backups: 45 files
```

### Step 5: Verify Results

```bash
# Check migrated notes
nb list --all

# Search to verify indexing
nb search "content from old notes"

# Open a few notes to verify frontmatter
nb manage
```

### Step 6: Clean Up

```bash
# If everything looks good, remove backups
find ~/Documents/nb -name "*.bak" -delete

# Remove temporary directory
rm -rf ~/temp-notes
```

## Frontmatter Transformation

### Before Migration

```markdown
# API Design Document

Some content here...
```

### After Migration

```markdown
---
id: 20240115-143022-api-design-document
title: API Design Document
tags: [architecture, api, design]
repository: my-project
branch: main
created: 2024-01-15T14:30:22-05:00
modified: 2024-01-15T14:30:22-05:00
---

# API Design Document

Some content here...
```

## Handling Special Cases

### Notes with Existing Frontmatter

By default, migration preserves existing frontmatter:

```bash
# Merge new fields with existing
nb migrate --all  # Adds missing fields only

# Force complete replacement
nb migrate --all --force  # Overwrites all frontmatter
```

### Non-Markdown Files

The migration tool only processes `.md` files. For other formats:

1. Convert to Markdown first (use pandoc or similar)
2. Then run migration

```bash
# Convert .txt files to .md
for f in *.txt; do 
  mv "$f" "${f%.txt}.md"
done

# Then migrate
nb migrate --all
```

### Large Note Collections

For thousands of notes:

```bash
# Process in batches by type
nb migrate ~/Documents/nb/repos/big-project/main/current --all
nb migrate ~/Documents/nb/repos/big-project/main/issues --all
nb migrate ~/Documents/nb/repos/big-project/main/architecture --all

# Use verbose mode to track progress
nb migrate --all --verbose
```

### Nested Type Structures

Migration respects nested types:

```bash
# Notes in issues/bugs/ get both tags
~/Documents/nb/repos/project/main/issues/bugs/login-error.md
# Results in: tags: [issues, bugs]
```

## Migration Report

The migration tool provides a detailed report:

```
MIGRATION REPORT
================
Workspace: my-project
Branch: main
Type: all

Files Processed: 127
Files Modified: 89
Files Skipped: 38

Operations:
  ✓ Titles fixed: 45
  ✓ IDs generated: 89
  ✓ Dates added: 67
  ✓ Tags created: 89
  ✓ Filenames standardized: 34
  ✓ SQLite indexed: 127

Errors: 0
Warnings: 2
  ⚠ Could not parse date from: old-note.md
  ⚠ Filename too long, truncated: very-long-note-title-that-exceeds-limits.md

Time: 2.4s
```

## Best Practices

### 1. Always Preview First

```bash
nb migrate --all --dry-run | tee migration-plan.txt
```

### 2. Backup Before Migration

```bash
# nb creates .bak files by default
# But also consider:
tar -czf notes-backup.tar.gz ~/Documents/nb
```

### 3. Migrate in Stages

Instead of `--all-workspaces`, migrate one at a time:

```bash
for workspace in $(nb workspace list | awk '{print $1}'); do
  echo "Migrating $workspace..."
  nb migrate --workspace "$workspace" --all
  read -p "Continue? (y/n) " -n 1 -r
  echo
  if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    break
  fi
done
```

### 4. Verify Search Index

After migration, ensure search works:

```bash
# Rebuild index if needed
nb migrate --index-sqlite

# Test search
nb search "test query" --all
```

### 5. Clean File Names First

If you have problematic filenames:

```bash
# Remove special characters before migration
for f in *.md; do
  newname=$(echo "$f" | tr ' ' '-' | tr -cd '[:alnum:]-._')
  [ "$f" != "$newname" ] && mv "$f" "$newname"
done
```

## Troubleshooting

### Issue: Migration Seems Stuck

For large collections, use verbose mode:
```bash
nb migrate --all --verbose
```

### Issue: Frontmatter Not Added

Check file permissions:
```bash
ls -la ~/Documents/nb/repos/project/main/current/
chmod 644 *.md
```

### Issue: Dates Are Wrong

Ensure files preserve timestamps when copying:
```bash
cp -p source/*.md destination/  # -p preserves timestamps
```

### Issue: Search Not Working After Migration

Rebuild the search index:
```bash
nb migrate --index-sqlite
# Or use doctor
nb doctor --fix
```

### Issue: Filename Conflicts

If standardization creates duplicate names:
```bash
# Migration will append numbers
note.md → 20240115-note.md
note.md → 20240115-note-2.md
```

## Advanced Migration Scripts

### Bulk Import with Metadata

```bash
#!/bin/bash
# import-with-metadata.sh

SOURCE_DIR="$1"
DEST_TYPE="$2"

for file in "$SOURCE_DIR"/*.md; do
  filename=$(basename "$file")
  title=$(grep -m1 "^# " "$file" | sed 's/^# //')
  
  # Create note with metadata
  nb new -t "$DEST_TYPE" "$title" --no-edit
  
  # Copy content (excluding first heading)
  tail -n +2 "$file" >> "$(nb context --path "$DEST_TYPE")/$(date +%Y%m%d)-${title// /-}.md"
done

# Run migration to standardize
nb migrate --all
```

### Notion Export Processing

```bash
#!/bin/bash
# process-notion-export.sh

# Notion exports have UUID suffixes
for file in *.md; do
  # Remove UUID suffix (last 32 chars before .md)
  newname=$(echo "$file" | sed 's/ [a-f0-9]\{32\}\.md$/.md/')
  mv "$file" "$newname"
done

# Fix Notion's image links
find . -name "*.md" -exec sed -i 's/!\[\](\(.*\))/![Image](\1)/g' {} \;

# Run migration
nb migrate --all
```

## Summary

The migration tool is essential for:
- Importing notes from other systems
- Standardizing existing notes
- Maintaining consistency across workspaces
- Enabling full-text search on all content
- Preserving note history and metadata

With proper use of migration options and following best practices, you can seamlessly integrate any Markdown notes into the `nb` ecosystem while maintaining their historical context and improving their discoverability.