package service

import (
	"strings"

	coreconfig "github.com/grovetools/core/config"
	"github.com/grovetools/core/tui/theme"
)

// DefaultNoteTypes provides the built-in configuration for "special" note types.
// User configurations in grove.yml can override these settings.
//
// DefaultExpand is false for every built-in type on purpose: opening a notebook
// should land on the workspace's group headings with their counts — an index —
// rather than on whichever groups happened to expand into hundreds of note
// rows. Users who want a group open on arrival set default_expand on it in
// their notebook config.
var DefaultNoteTypes = map[string]*coreconfig.NoteTypeConfig{
	"inbox": {
		Icon:          theme.IconNoteInbox,
		IconColor:     "orange",
		DefaultExpand: false,
		SortOrder:     10,
		Description:   "Default location for new notes.",
	},
	"issues": {
		Icon:          theme.IconNoteIssues,
		IconColor:     "red",
		DefaultExpand: false,
		SortOrder:     11,
		Description:   "Notes related to bugs or issues.",
	},
	"plans": {
		Icon:          theme.IconPlan,
		IconColor:     "blue",
		DefaultExpand: false,
		SortOrder:     13,
		Description:   "Directory for structured project plans.",
	},
	"skills": {
		Icon:        theme.IconBuild,
		IconColor:   "orange",
		SortOrder:   15,
		Description: "Agent skills for automation and integration.",
	},
	"in_progress": {
		Icon:          theme.IconNoteInProgress,
		IconColor:     "blue",
		DefaultExpand: false,
		SortOrder:     20,
		Description:   "Notes for tasks currently being worked on.",
	},
	"review": {
		Icon:          theme.IconNoteReview,
		IconColor:     "pink",
		DefaultExpand: false,
		SortOrder:     30,
		Description:   "Notes or PRs ready for review.",
	},
	"completed": {
		Icon:          theme.IconNoteCompleted,
		IconColor:     "green",
		DefaultExpand: false,
		SortOrder:     999,
		Description:   "Completed work and historical notes.",
	},
	"docs": {
		Icon:        theme.IconDocs,
		IconColor:   "orange",
		SortOrder:   40,
		Description: "Documentation and reference materials.",
	},
	"learn": {
		Icon:        theme.IconSchool,
		IconColor:   "orange",
		SortOrder:   50,
		Description: "Learning materials and educational content.",
	},
	"daily": {
		Icon:        theme.IconCalendar,
		SortOrder:   5,
		Description: "Daily notes and journal entries.",
	},
	"github-issues": {
		Icon:        theme.IconIssueOpened,
		IconColor:   "red",
		SortOrder:   12,
		Description: "GitHub issues.",
	},
	"github-prs": {
		Icon:        theme.IconPullRequest,
		IconColor:   "pink",
		SortOrder:   31,
		Description: "GitHub pull requests.",
	},
	".archive": {
		Icon:        theme.IconArchive,
		Description: "Archived items.",
	},
	".closed": {
		Icon:        theme.IconArchive,
		Description: "Closed items.",
	},
	".artifacts": {
		Icon:        theme.IconDocs,
		Description: "Generated artifacts and outputs.",
	},
	"quick": {
		Icon:        theme.IconClockFast,
		Description: "Quick notes and scratch space.",
	},
	"prompts": {
		Icon:        theme.IconLightbulb,
		Description: "AI prompts and templates.",
	},
	"blog": {
		Icon:        theme.IconRss,
		Description: "Blog posts and articles.",
	},
	"architecture": {
		Icon:        theme.IconArchitecture,
		Description: "Architecture documentation and design.",
	},
	"todos": {
		Icon:        theme.IconChecklist,
		Description: "Task lists and todos.",
	},
	"concepts": {
		Icon:          theme.IconLightbulb,
		IconColor:     "cyan",
		SortOrder:     60,
		DefaultExpand: false,
		Description:   "Project concepts and architectural memory.",
	},
	"context": {
		Icon:          theme.IconFolderEye,
		SortOrder:     95,
		DefaultExpand: false,
		Description:   "Context rules and presets.",
	},
}

// ResolveNoteTypes builds the effective note-type registry: the built-in
// DefaultNoteTypes overlaid with the default notebook's user-defined types from
// grove config. It is what Service.NoteTypes is built from, exported so
// out-of-process consumers that only need the presentation metadata (icons,
// sort order) can resolve it without constructing a full Service — the drawer's
// notes summary in treemux reads it to render nb's own group icons.
//
// The returned map is always freshly allocated with copied values, so callers
// may mutate it without corrupting the package-level defaults.
func ResolveNoteTypes(coreCfg *coreconfig.Config) map[string]*coreconfig.NoteTypeConfig {
	final := make(map[string]*coreconfig.NoteTypeConfig, len(DefaultNoteTypes))
	for name, cfg := range DefaultNoteTypes {
		copyCfg := *cfg
		final[name] = &copyCfg
	}

	if coreCfg == nil || coreCfg.Notebooks == nil || coreCfg.Notebooks.Definitions == nil {
		return final
	}
	defaultNotebookName := "default" //nolint:goconst
	if coreCfg.Notebooks.Rules != nil && coreCfg.Notebooks.Rules.Default != "" {
		defaultNotebookName = coreCfg.Notebooks.Rules.Default
	}
	notebook, ok := coreCfg.Notebooks.Definitions[defaultNotebookName]
	if !ok || notebook == nil || notebook.Types == nil {
		return final
	}
	for name, userCfg := range notebook.Types {
		if userCfg == nil {
			continue
		}
		existing, exists := final[name]
		if !exists {
			// User defined a completely new type.
			copyCfg := *userCfg
			final[name] = &copyCfg
			continue
		}
		if userCfg.Icon != "" {
			existing.Icon = userCfg.Icon
		}
		if userCfg.IconColor != "" {
			existing.IconColor = userCfg.IconColor
		}
		if userCfg.DefaultExpand {
			existing.DefaultExpand = userCfg.DefaultExpand
		}
		if userCfg.SortOrder != 0 {
			existing.SortOrder = userCfg.SortOrder
		}
		if userCfg.Description != "" {
			existing.Description = userCfg.Description
		}
		if userCfg.TemplatePath != "" {
			existing.TemplatePath = userCfg.TemplatePath
		}
		if userCfg.FilenameFormat != "" {
			existing.FilenameFormat = userCfg.FilenameFormat
		}
	}
	return final
}

// GroupIcon returns the icon for a note group, matching the browser tree's own
// resolution: an exact full-path match wins, a nested organizational
// subdirectory falls back to the generic folder icon, and a top-level group
// falls back to its base type's icon.
func GroupIcon(groupName string, noteTypes map[string]*coreconfig.NoteTypeConfig) string {
	if typeConfig, ok := noteTypes[groupName]; ok && typeConfig.Icon != "" {
		return typeConfig.Icon
	}
	parts := strings.Split(groupName, "/")
	if len(parts) > 1 {
		return theme.IconFolder
	}
	if len(parts) > 0 {
		if typeConfig, ok := noteTypes[parts[0]]; ok && typeConfig.Icon != "" {
			return typeConfig.Icon
		}
	}
	return theme.IconFolder
}

// GroupSortOrder returns the configured display order for a top-level note
// group, or a large sentinel for groups with no configured order so they sort
// after the known ones (matching the tree's own trailing placement).
func GroupSortOrder(groupName string, noteTypes map[string]*coreconfig.NoteTypeConfig) int {
	const unranked = 1 << 20
	base := groupName
	if i := strings.Index(base, "/"); i >= 0 {
		base = base[:i]
	}
	if cfg, ok := noteTypes[base]; ok && cfg.SortOrder != 0 {
		return cfg.SortOrder
	}
	return unranked
}
