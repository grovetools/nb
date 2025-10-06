package workspace

// GetNotePath returns the path for a note in this workspace.
// This is a helper function that can be used independently.
func GetNotePath(w *Workspace, noteType, branch string) string {
	return w.GetNotePath(noteType, branch)
}

// EnsureDirectories creates necessary directories for this workspace.
// This is a helper function that can be used independently.
func EnsureDirectories(w *Workspace, noteType, branch string) error {
	return w.EnsureDirectories(noteType, branch)
}

// GetNotebookDir returns the notebook directory for a workspace, with fallback.
func GetNotebookDir(w *Workspace) string {
	if w.NotebookDir != "" {
		return w.NotebookDir
	}
	return GetDefaultNotebookDir()
}
