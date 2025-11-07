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
