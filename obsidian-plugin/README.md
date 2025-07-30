# NB Integration for Obsidian

This plugin integrates the NB note system with Obsidian, providing a sidebar view to browse and manage your NB notes.

## Features

- Browse repositories and branches
- View notes in a clean table format
- Quick note creation
- Archive notes
- Sync with NB repositories

## Installation

1. Clone this repository into your vault's `.obsidian/plugins/` directory
2. Run `npm install` to install dependencies
3. Run `npm run dev` to build the plugin
4. Enable the plugin in Obsidian's settings

## Development

- `npm run dev` - Build and watch for changes
- `npm run build` - Build for production

## Usage

1. Open the NB Notes view from the ribbon icon or command palette
2. Configure your NB executable path in settings
3. Select repository and branch from the dropdowns
4. Click on notes to open them in Obsidian

## Requirements

- NB command line tool must be installed and accessible
- Obsidian 1.5.0 or higher