package service

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/grovetools/nb/pkg/frontmatter"
	"github.com/grovetools/nb/pkg/tree"
)

// newItemFromFile creates a tree.Item from a given file path.
// It determines the item's type and extracts relevant metadata.
func (s *Service) newItemFromFile(path string, info os.FileInfo) (*tree.Item, error) {
	item := &tree.Item{
		Path:     path,
		Name:     info.Name(),
		IsDir:    false,
		ModTime:  info.ModTime(),
		Metadata: make(map[string]interface{}),
	}

	// Determine item type and populate metadata
	if strings.HasSuffix(info.Name(), ".md") {
		// It's a markdown note, parse for frontmatter
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		contentStr := string(content)
		fm, _, err := frontmatter.Parse(contentStr)

		item.Type = tree.TypeNote
		if err == nil && fm != nil {
			// Populate metadata from frontmatter
			item.Metadata["Title"] = fm.Title
			item.Metadata["Tags"] = fm.Tags
			item.Metadata["ID"] = fm.ID
			item.Metadata["PlanRef"] = fm.PlanRef
			if fm.Remote != nil {
				item.Metadata["RemoteState"] = fm.Remote.State
			}
			// Use frontmatter timestamps if available
			if created, err := frontmatter.ParseTimestamp(fm.Created); err == nil {
				item.Metadata["Created"] = created
			}
			if modified, err := frontmatter.ParseTimestamp(fm.Modified); err == nil {
				item.ModTime = modified
			}
		} else {
			// No frontmatter, extract title from H1 or filename
			title := extractTitle(contentStr)
			if title == "Untitled" {
				title = strings.TrimSuffix(info.Name(), ".md")
			}
			item.Metadata["Title"] = title
		}

	} else if strings.Contains(path, ".artifacts") {
		item.Type = tree.TypeArtifact
		item.Metadata["Title"] = strings.TrimSuffix(info.Name(), filepath.Ext(info.Name()))
	} else {
		item.Type = tree.TypeGeneric
		item.Metadata["Title"] = info.Name()
		item.Metadata["Extension"] = strings.TrimPrefix(filepath.Ext(info.Name()), ".")
	}

	// Common metadata for all files
	item.Metadata["Path"] = path // Store full path in metadata for easy access in TUI

	return item, nil
}
