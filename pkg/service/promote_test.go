package service

import (
	"testing"
)

func TestStripFrontmatterBlock(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "valid frontmatter block",
			input: `---
id: test-123
title: Test Note
---

# Content

This is the body.`,
			expected: "\n# Content\n\nThis is the body.",
		},
		{
			name: "no frontmatter",
			input: `# Content

Just body content.`,
			expected: `# Content

Just body content.`,
		},
		{
			name: "broken YAML in frontmatter",
			input: `---
id: broken-yaml-note
title: treemux: drag-select offset ~2 lines; copy banner reflows
tags: [issues, grovetools]
created: 2023-01-01 10:00:00
modified: 2023-01-01 10:00:00
---

# Issue Description

This note has a colon in the title which, when unquoted, creates invalid YAML.`,
			expected: "\n# Issue Description\n\nThis note has a colon in the title which, when unquoted, creates invalid YAML.",
		},
		{
			name: "frontmatter with empty body",
			input: `---
id: test
title: Test
---

`,
			expected: "\n",
		},
		{
			name: "only opening delimiter",
			input: `---
incomplete frontmatter`,
			expected: `---
incomplete frontmatter`,
		},
		{
			name: "frontmatter with multiple --- in body",
			input: `---
id: test
title: Test
---

# Content

Some text with --- separator

More content`,
			expected: "\n# Content\n\nSome text with --- separator\n\nMore content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripFrontmatterBlock(tt.input)
			if got != tt.expected {
				t.Errorf("stripFrontmatterBlock() = %q, want %q", got, tt.expected)
			}
		})
	}
}
