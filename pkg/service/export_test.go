package service

import coremodels "github.com/grovetools/core/pkg/models"

// StubDaemonNotifierForTests replaces the fire-and-forget daemon notifier
// with fn and returns a restore func. It exists for the external
// service_test package (the CLI contract tests drive the real cobra commands
// end to end, and without this the note events they trigger would try to
// auto-start a real daemon from inside `go test`). Test-binary only: files
// named *_test.go never ship.
func StubDaemonNotifierForTests(fn func(coremodels.NoteEvent)) (restore func()) {
	orig := notifyDaemonNoteEvent
	notifyDaemonNoteEvent = fn
	return func() { notifyDaemonNoteEvent = orig }
}
