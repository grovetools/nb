package service

import (
	"testing"

	coreconfig "github.com/grovetools/core/config"
)

// A notebook opens as an index: every built-in group starts folded, so the
// first screen is group headings with counts rather than whichever groups
// expanded into hundreds of note rows.
func TestBuiltInNoteTypesStartFolded(t *testing.T) {
	for name, cfg := range DefaultNoteTypes {
		if cfg.DefaultExpand {
			t.Errorf("built-in note type %q has DefaultExpand=true; first open must show headings only", name)
		}
	}
}

// Expanding on arrival stays available per group through notebook config.
func TestUserConfigCanRestoreDefaultExpand(t *testing.T) {
	cfg := &coreconfig.Config{
		Notebooks: &coreconfig.NotebooksConfig{
			Rules: &coreconfig.NotebookRules{Default: "default"},
			Definitions: map[string]*coreconfig.Notebook{
				"default": {
					Types: map[string]*coreconfig.NoteTypeConfig{
						"plans": {DefaultExpand: true},
					},
				},
			},
		},
	}

	resolved := ResolveNoteTypes(cfg)
	if !resolved["plans"].DefaultExpand {
		t.Error("notebook config did not re-enable DefaultExpand for plans")
	}
	if resolved["inbox"].DefaultExpand {
		t.Error("unconfigured type inbox should stay folded")
	}
	// The package-level defaults must not be mutated by resolution.
	if DefaultNoteTypes["plans"].DefaultExpand {
		t.Error("ResolveNoteTypes leaked the override into DefaultNoteTypes")
	}
}
