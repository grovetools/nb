package service

import (
	coreconfig "github.com/mattsolo1/grove-core/config"
	"github.com/mattsolo1/grove-core/tui/theme"
)

// DefaultNoteTypes provides the built-in configuration for "special" note types.
// User configurations in grove.yml can override these settings.
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
	"in_progress": {
		Icon:          theme.IconNoteInProgress,
		IconColor:     "blue",
		DefaultExpand: true,
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
	"plans": {
		Icon:          theme.IconPlan,
		IconColor:     "blue",
		DefaultExpand: true,
		SortOrder:     15,
		Description:   "Directory for structured project plans.",
	},
	"docs": {
		Icon:       theme.IconDocs,
		IconColor:  "orange",
		SortOrder:  40,
		Description: "Documentation and reference materials.",
	},
	"learn": {
		Icon:       theme.IconSchool,
		IconColor:  "orange",
		SortOrder:  50,
		Description: "Learning materials and educational content.",
	},
	"daily": {
		Icon:       theme.IconCalendar,
		SortOrder:  5,
		Description: "Daily notes and journal entries.",
	},
	"github-issues": {
		Icon:       theme.IconIssueOpened,
		IconColor:  "red",
		SortOrder:  12,
		Description: "GitHub issues.",
	},
	"github-prs": {
		Icon:       theme.IconPullRequest,
		IconColor:  "pink",
		SortOrder:  31,
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
}
