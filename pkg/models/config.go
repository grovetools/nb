package models

// FilenameFormat defines how files should be named
type FilenameFormat string

const (
	// FilenameFormatTimestampTitle uses YYYYMMDD-HHMMSS-title.md
	FilenameFormatTimestampTitle FilenameFormat = "timestamp-title"

	// FilenameFormatDateTitle uses YYYYMMDD-title.md
	FilenameFormatDateTitle FilenameFormat = "date-title"

	// FilenameFormatTitle uses title.md
	FilenameFormatTitle FilenameFormat = "title"

	// FilenameFormatTimestamp uses YYYYMMDD-HHMMSS.md
	FilenameFormatTimestamp FilenameFormat = "timestamp"
)

// NoteTypeConfig defines configuration for each note type
type NoteTypeConfig struct {
	FilenameFormat FilenameFormat
	Template       string
}

// DefaultNoteTypeConfigs provides sensible defaults
var DefaultNoteTypeConfigs = map[NoteType]NoteTypeConfig{
	NoteTypeDaily: {
		FilenameFormat: FilenameFormatTimestamp, // Just 20250108-143022.md
	},
	NoteTypeLLM: {
		FilenameFormat: FilenameFormatTimestampTitle, // Full timestamp for chat sessions
	},
	NoteTypeCurrent: {
		FilenameFormat: FilenameFormatTitle, // Just the-title.md
	},
	NoteTypeLearn: {
		FilenameFormat: FilenameFormatTitle, // learning-golang.md
	},
	NoteTypeIssues: {
		FilenameFormat: FilenameFormatDateTitle, // 20250108-bug-in-auth.md
	},
	NoteTypeArchitecture: {
		FilenameFormat: FilenameFormatTitle, // system-design.md
	},
	NoteTypeTodos: {
		FilenameFormat: FilenameFormatTitle, // project-tasks.md
	},
}
