package search

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/mattn/go-sqlite3"

	"github.com/mattsolo1/nb/pkg/models"
	"github.com/mattsolo1/nb/pkg/workspace"
)

// Index manages the search index
type Index struct {
	db     *sql.DB
	useFTS bool
}

// NewIndex creates a new search index
func NewIndex(dbPath string) (*Index, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	idx := &Index{db: db}
	if err := idx.init(); err != nil {
		return nil, err
	}

	return idx, nil
}

// init creates the database schema
func (idx *Index) init() error {
	// First, check if FTS5 is available
	idx.useFTS = idx.checkFTS5Support()

	// Create metadata table first (always needed)
	metaSchema := `
	CREATE TABLE IF NOT EXISTS notes_meta (
		path TEXT PRIMARY KEY,
		workspace TEXT,
		branch TEXT,
		type TEXT,
		created_at TIMESTAMP,
		modified_at TIMESTAMP,
		word_count INTEGER,
		has_todos BOOLEAN,
		is_archived BOOLEAN
	);
	
	CREATE INDEX IF NOT EXISTS idx_notes_meta_workspace ON notes_meta(workspace);
	CREATE INDEX IF NOT EXISTS idx_notes_meta_type ON notes_meta(type);
	CREATE INDEX IF NOT EXISTS idx_notes_meta_archived ON notes_meta(is_archived);
	`

	if _, err := idx.db.Exec(metaSchema); err != nil {
		return err
	}

	// Add title and content columns if they don't exist (for backward compatibility)
	// Ignore errors as columns may already exist
	_, _ = idx.db.Exec("ALTER TABLE notes_meta ADD COLUMN title TEXT")
	_, _ = idx.db.Exec("ALTER TABLE notes_meta ADD COLUMN content TEXT")

	// Create indexes for the new columns (ignore errors if already exist)
	_, _ = idx.db.Exec("CREATE INDEX IF NOT EXISTS idx_notes_meta_title ON notes_meta(title)")
	_, _ = idx.db.Exec("CREATE INDEX IF NOT EXISTS idx_notes_meta_content ON notes_meta(content)")

	// Create FTS table if supported
	if idx.useFTS {
		ftsSchema := `
		CREATE VIRTUAL TABLE IF NOT EXISTS notes_fts USING fts5(
			path UNINDEXED,
			workspace,
			branch,
			type,
			title,
			content,
			tokenize = 'porter unicode61'
		);
		`

		if _, err := idx.db.Exec(ftsSchema); err != nil {
			// If FTS creation fails, disable FTS and continue
			idx.useFTS = false
		}
	}

	return nil
}

// checkFTS5Support checks if FTS5 module is available
func (idx *Index) checkFTS5Support() bool {
	// Try to create a test FTS5 table to check if it's supported
	_, err := idx.db.Exec("CREATE VIRTUAL TABLE IF NOT EXISTS fts5_test USING fts5(content)")
	if err != nil {
		return false
	}

	// Clean up test table
	_, _ = idx.db.Exec("DROP TABLE IF EXISTS fts5_test")
	return true
}

// IndexNote indexes or reindexes a note
func (idx *Index) IndexNote(note *models.Note) error {
	tx, err := idx.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// Delete existing entries from FTS table if using FTS
	if idx.useFTS {
		_, err = tx.Exec("DELETE FROM notes_fts WHERE path = ?", note.Path)
		if err != nil {
			return err
		}
	}

	// Delete existing entries from metadata table
	_, err = tx.Exec("DELETE FROM notes_meta WHERE path = ?", note.Path)
	if err != nil {
		return err
	}

	// Insert into FTS table if using FTS
	if idx.useFTS {
		_, err = tx.Exec(`
			INSERT INTO notes_fts (path, workspace, branch, type, title, content)
			VALUES (?, ?, ?, ?, ?, ?)
		`, note.Path, note.Workspace, note.Branch, note.Type, note.Title, note.Content)
		if err != nil {
			return err
		}
	}

	// Insert into metadata table (always)
	_, err = tx.Exec(`
		INSERT INTO notes_meta (
			path, workspace, branch, type, title, content, created_at, modified_at,
			word_count, has_todos, is_archived
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, note.Path, note.Workspace, note.Branch, note.Type, note.Title, note.Content,
		note.CreatedAt, note.ModifiedAt, note.WordCount,
		note.HasTodos, note.IsArchived)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// Options for searching
type Options struct {
	Workspace *workspace.Workspace
	Type      string
	Limit     int
}

// Search performs a full-text search
func (idx *Index) Search(query string, opts *Options) ([]*models.Note, error) {
	if opts == nil {
		opts = &Options{Limit: 50}
	}
	if opts.Limit == 0 {
		opts.Limit = 50
	}

	if idx.useFTS {
		return idx.searchWithFTS(query, opts)
	}
	return idx.searchWithoutFTS(query, opts)
}

// searchWithFTS performs search using FTS5
func (idx *Index) searchWithFTS(query string, opts *Options) ([]*models.Note, error) {
	// Build the query
	var conditions []string
	var args []any

	// Always search non-archived by default
	conditions = append(conditions, "m.is_archived = 0")

	if opts.Workspace != nil {
		conditions = append(conditions, "m.workspace = ?")
		args = append(args, opts.Workspace.Name)
	}

	if opts.Type != "" {
		conditions = append(conditions, "m.type = ?")
		args = append(args, opts.Type)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Perform FTS search
	searchQuery := fmt.Sprintf(`
		SELECT 
			f.path, f.workspace, f.branch, f.type, f.title,
			m.created_at, m.modified_at, m.word_count, m.has_todos, m.is_archived,
			snippet(notes_fts, 5, '<match>', '</match>', '...', 32) as snippet
		FROM notes_fts f
		JOIN notes_meta m ON f.path = m.path
		%s AND notes_fts MATCH ?
		ORDER BY rank
		LIMIT ?
	`, whereClause)

	args = append(args, query, opts.Limit)

	rows, err := idx.db.Query(searchQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*models.Note
	for rows.Next() {
		note := &models.Note{}
		var snippet string

		err := rows.Scan(
			&note.Path, &note.Workspace, &note.Branch, &note.Type, &note.Title,
			&note.CreatedAt, &note.ModifiedAt, &note.WordCount, &note.HasTodos, &note.IsArchived,
			&snippet,
		)
		if err != nil {
			return nil, err
		}

		results = append(results, note)
	}

	return results, nil
}

// searchWithoutFTS performs search using LIKE queries on metadata table
func (idx *Index) searchWithoutFTS(query string, opts *Options) ([]*models.Note, error) {
	// Build the query
	var conditions []string
	var args []any

	// Always search non-archived by default
	conditions = append(conditions, "is_archived = 0")

	if opts.Workspace != nil {
		conditions = append(conditions, "workspace = ?")
		args = append(args, opts.Workspace.Name)
	}

	if opts.Type != "" {
		conditions = append(conditions, "type = ?")
		args = append(args, opts.Type)
	}

	// Add search conditions for title and content
	searchPattern := "%" + strings.ReplaceAll(query, " ", "%") + "%"
	conditions = append(conditions, "(title LIKE ? OR content LIKE ?)")
	args = append(args, searchPattern, searchPattern)

	whereClause := "WHERE " + strings.Join(conditions, " AND ")

	// Perform LIKE search
	searchQuery := fmt.Sprintf(`
		SELECT 
			path, workspace, branch, type, title,
			created_at, modified_at, word_count, has_todos, is_archived
		FROM notes_meta
		%s
		ORDER BY modified_at DESC
		LIMIT ?
	`, whereClause)

	args = append(args, opts.Limit)

	rows, err := idx.db.Query(searchQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*models.Note
	for rows.Next() {
		note := &models.Note{}

		err := rows.Scan(
			&note.Path, &note.Workspace, &note.Branch, &note.Type, &note.Title,
			&note.CreatedAt, &note.ModifiedAt, &note.WordCount, &note.HasTodos, &note.IsArchived,
		)
		if err != nil {
			return nil, err
		}

		results = append(results, note)
	}

	return results, nil
}

// RemoveNote removes a note from the index
func (idx *Index) RemoveNote(path string) error {
	tx, err := idx.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// Remove from FTS table if using FTS
	if idx.useFTS {
		_, err = tx.Exec("DELETE FROM notes_fts WHERE path = ?", path)
		if err != nil {
			return err
		}
	}

	// Remove from metadata table
	_, err = tx.Exec("DELETE FROM notes_meta WHERE path = ?", path)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// Close closes the index
func (idx *Index) Close() error {
	return idx.db.Close()
}
