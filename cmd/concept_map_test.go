package cmd

import (
	"strings"
	"testing"

	"github.com/grovetools/nb/pkg/service"
)

// The backend and id checks run before any service access, so a nil service
// is enough to exercise the rejection paths.
func executeConceptMapNew(t *testing.T, args ...string) error {
	t.Helper()
	var svc *service.Service
	workspaceOverride := ""
	cmd := newConceptMapNewCmd(&svc, &workspaceOverride)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs(args)
	return cmd.Execute()
}

func TestConceptMapNewRejectsUnknownBackend(t *testing.T) {
	err := executeConceptMapNew(t, "payments", "--backend", "mermaid")
	if err == nil || !strings.Contains(err.Error(), "unsupported backend") {
		t.Fatalf("error = %v, want unsupported backend", err)
	}
}

func TestConceptMapNewRejectsInvalidLikeC4ID(t *testing.T) {
	for _, id := range []string{"default", "pay.ments", "pay@ments", "pay#ments"} {
		err := executeConceptMapNew(t, id)
		if err == nil || !strings.Contains(err.Error(), "invalid concept map id") &&
			!strings.Contains(err.Error(), "must not be") {
			t.Fatalf("id %q: error = %v, want id validation error", id, err)
		}
	}
}
