package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func executeConceptPath(t *testing.T, resolver conceptPathResolver, args ...string) (string, error) {
	t.Helper()
	cmd := newConceptPathCmdWithResolver(resolver)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return strings.TrimSpace(out.String()), err
}

func TestConceptPathCommandAcceptsQualifiedReference(t *testing.T) {
	var gotRef string
	out, err := executeConceptPath(t, func(ref string) (string, error) {
		gotRef = ref
		return "/notebooks/workspaces/core.v2/concepts/workspace-model", nil
	}, "core.v2:workspace-model", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if gotRef != "core.v2:workspace-model" {
		t.Fatalf("resolver ref = %q", gotRef)
	}
	var envelope map[string]string
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("invalid JSON: %v: %s", err, out)
	}
	if envelope["path"] != "/notebooks/workspaces/core.v2/concepts/workspace-model" || len(envelope) != 1 {
		t.Fatalf("legacy path schema changed: %#v", envelope)
	}
}

func TestConceptPathCommandPreservesUnqualifiedHumanOutput(t *testing.T) {
	out, err := executeConceptPath(t, func(ref string) (string, error) {
		if ref != "local-concept" {
			t.Fatalf("resolver ref = %q", ref)
		}
		return "/notebooks/workspaces/current/concepts/local-concept", nil
	}, "local-concept")
	if err != nil {
		t.Fatal(err)
	}
	if out != "/notebooks/workspaces/current/concepts/local-concept" {
		t.Fatalf("human output = %q", out)
	}
}

func TestConceptPathCommandPropagatesUnknownReferenceErrors(t *testing.T) {
	want := errors.New("workspace 'missing' not found")
	_, err := executeConceptPath(t, func(string) (string, error) { return "", want }, "missing:concept", "--json")
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}
