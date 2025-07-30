package migration

import (
	"time"
)

type MigrationScope struct {
	Context   string
	Global    bool
	Workspace string
	All       bool
}

type MigrationIssue struct {
	Type        string
	Description string
	Field       string
	Current     interface{}
	Expected    interface{}
}

type MigrationReport struct {
	TotalFiles       int
	ProcessedFiles   int
	MigratedFiles    int
	SkippedFiles     int
	FailedFiles      int
	CreatedFiles     int
	DeletedFiles     int
	IssuesFixed      int
	Errors           []string
	ProcessingErrors map[string]error
	StartTime        time.Time
	EndTime          time.Time
}

type MigrationOptions struct {
	Scope      MigrationScope
	DryRun     bool
	Verbose    bool
	ShowReport bool
	NoBackup   bool
}

func NewMigrationReport() *MigrationReport {
	return &MigrationReport{
		ProcessingErrors: make(map[string]error),
		StartTime:        time.Now(),
	}
}

func (r *MigrationReport) AddError(file string, err error) {
	r.ProcessingErrors[file] = err
	r.FailedFiles++
}

func (r *MigrationReport) Complete() {
	r.EndTime = time.Now()
}

func (r *MigrationReport) Duration() time.Duration {
	return r.EndTime.Sub(r.StartTime)
}
