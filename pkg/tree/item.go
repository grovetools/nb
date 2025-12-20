package tree

import (
	"time"
)

// ItemType categorizes the different kinds of items in the notebook tree.
type ItemType string

const (
	TypeNote      ItemType = "note"
	TypePlan      ItemType = "plan"      // A plan *directory*
	TypeArtifact  ItemType = "artifact"  // A generated file, e.g., briefing.xml
	TypeGeneric   ItemType = "generic"   // A generic file, e.g., notes.txt
	TypeWorkspace ItemType = "workspace" // A workspace root directory
	TypeGroup     ItemType = "group"     // A generic grouping directory, e.g., 'inbox', 'meetings'
)

// Item represents a single node in the notebook's file tree. It can be a file or a directory.
type Item struct {
	Path     string
	Name     string
	IsDir    bool
	ModTime  time.Time
	Type     ItemType
	Metadata map[string]interface{} // For type-specific data like Title, Tags, Status, etc.

	// Hierarchy
	Parent   *Item
	Children []*Item
}
