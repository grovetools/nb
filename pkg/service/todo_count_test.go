package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCountTodos(t *testing.T) {
	tests := []struct {
		name                        string
		content                     string
		wantOpen, wantDone, wantCxl int
	}{
		{
			name:     "open done and cancelled dash bullets",
			content:  "- [ ] open one\n- [x] done lower\n- [X] done upper\n- [-] cancelled\n- [ ] open two\n",
			wantOpen: 2, wantDone: 2, wantCxl: 1,
		},
		{
			name:     "star bullet variants",
			content:  "* [ ] open\n* [x] done\n* [X] done too\n* [-] gone\n",
			wantOpen: 1, wantDone: 2, wantCxl: 1,
		},
		{
			name:     "indented items are counted",
			content:  "- [ ] top\n  - [x] nested done\n\t- [-] nested cancelled\n",
			wantOpen: 1, wantDone: 1, wantCxl: 1,
		},
		{
			name:     "code fence lines are excluded",
			content:  "- [ ] real\n```markdown\n- [ ] fenced example\n- [x] fenced done\n```\n- [x] real done\n",
			wantOpen: 1, wantDone: 1, wantCxl: 0,
		},
		{
			name:     "non-todo lines are ignored",
			content:  "# Title\n\nplain text - [ ]not a todo (no space)\n-[x] missing bullet space\n[ ] bare brackets\n",
			wantOpen: 0, wantDone: 0, wantCxl: 0,
		},
		{
			name:     "empty content",
			content:  "",
			wantOpen: 0, wantDone: 0, wantCxl: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			open, done, cancelled := CountTodos(tt.content)
			assert.Equal(t, tt.wantOpen, open, "open")
			assert.Equal(t, tt.wantDone, done, "done")
			assert.Equal(t, tt.wantCxl, cancelled, "cancelled")
		})
	}
}

func TestParseNotePopulatesTodoCounts(t *testing.T) {
	tempDir := t.TempDir()
	noteDir := filepath.Join(tempDir, "nb", "repos", "test-repo", "main", "current")
	require.NoError(t, os.MkdirAll(noteDir, 0o755))

	notePath := filepath.Join(noteDir, "todos.md")
	content := "# Todos\n\n- [ ] open\n- [x] done\n- [X] done caps\n- [-] cancelled\n"
	require.NoError(t, os.WriteFile(notePath, []byte(content), 0o644))

	note, err := ParseNote(notePath)
	require.NoError(t, err)

	assert.Equal(t, 1, note.TodoOpen)
	assert.Equal(t, 2, note.TodoDone)
	assert.Equal(t, 1, note.TodoCancelled)
	assert.True(t, note.HasTodos)
}
