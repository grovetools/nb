package service

import (
	"strings"
	"testing"
)

func TestConceptOverviewBodyIncludesRole(t *testing.T) {
	body := conceptOverviewBody("Rate Limiter")
	role := strings.Index(body, "## Role\n")
	summary := strings.Index(body, "## Summary\n")
	if role < 0 {
		t.Fatal("new concept overview must include ## Role")
	}
	if summary < 0 || role > summary {
		t.Fatalf("Role must precede Summary:\n%s", body)
	}
}
