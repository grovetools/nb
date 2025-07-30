package migration

import (
	"github.com/mattsolo1/nb/pkg/frontmatter"
)

// Maintain compatibility aliases for existing code
type Frontmatter = frontmatter.Frontmatter

var (
	ParseFrontmatter            = frontmatter.Parse
	BuildContentWithFrontmatter = frontmatter.BuildContent
	FormatTimestamp             = frontmatter.FormatTimestamp
)
