package cmd

import (
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grovetools/nb/pkg/service"
)

// documentSyncVerbs is the notebook document-sync family: daemon-proxied
// commands that have nothing to do with forge mirroring.
var documentSyncVerbs = []string{"adopt", "conflicts", "history", "incoming", "restore"}

// syncTestTree builds the command tree exactly as main.go registers it, so
// these tests pin the real CLI surface rather than a hand-built stand-in.
func syncTestTree(t *testing.T) *cobra.Command {
	t.Helper()
	var svc *service.Service
	var workspaceOverride string

	root := &cobra.Command{Use: "nb"}
	root.AddCommand(NewRemoteCmd(&svc, &workspaceOverride))
	root.AddCommand(NewNotebookSyncCmd(&svc, &workspaceOverride))
	return root
}

func findCmd(t *testing.T, root *cobra.Command, path ...string) *cobra.Command {
	t.Helper()
	found, _, err := root.Find(path)
	require.NoError(t, err, "resolving %q", strings.Join(path, " "))
	require.Equal(t, path[len(path)-1], found.Name(),
		"%q did not resolve to a command of that name", strings.Join(path, " "))
	return found
}

func subcommandNames(cmd *cobra.Command) []string {
	var names []string
	for _, c := range cmd.Commands() {
		names = append(names, c.Name())
	}
	sort.Strings(names)
	return names
}

// TestRemoteSyncIsTheCanonicalMirroringEntry pins that forge mirroring lives at
// `nb remote sync` and is runnable there.
func TestRemoteSyncIsTheCanonicalMirroringEntry(t *testing.T) {
	root := syncTestTree(t)

	remoteSync := findCmd(t, root, "remote", "sync")
	assert.NotNil(t, remoteSync.RunE, "`nb remote sync` must itself run the mirror")
	assert.NotNil(t, remoteSync.Flags().Lookup("provider"),
		"`nb remote sync` must keep its --provider flag")
}

// TestNotebookSyncFamilyIsAtRoot pins the other half of the split: the
// document-sync verbs are reachable as `nb sync <sub>`, which is the spelling
// the ecosystem's own operator docs have always used.
func TestNotebookSyncFamilyIsAtRoot(t *testing.T) {
	root := syncTestTree(t)

	sync := findCmd(t, root, "sync")
	assert.Equal(t, documentSyncVerbs, subcommandNames(sync))

	// Bare `nb sync` must not mirror a forge — that would rebuild the very
	// collision this split exists to remove.
	assert.Nil(t, sync.RunE, "bare `nb sync` must print help, not run the mirror")
	assert.Nil(t, sync.Run, "bare `nb sync` must print help, not run the mirror")
	assert.Nil(t, sync.Flags().Lookup("provider"))

	for _, verb := range documentSyncVerbs {
		sub := findCmd(t, root, "sync", verb)
		assert.Empty(t, sub.Deprecated, "`nb sync %s` is the canonical spelling", verb)
	}
}

// TestLegacyRemoteSyncSpellingsStillResolve is the "remove nothing" guard:
// every pre-split `nb remote sync <sub>` invocation must keep working, and say
// so exactly once.
func TestLegacyRemoteSyncSpellingsStillResolve(t *testing.T) {
	root := syncTestTree(t)

	for _, verb := range documentSyncVerbs {
		t.Run(verb, func(t *testing.T) {
			legacy := findCmd(t, root, "remote", "sync", verb)

			require.NotEmpty(t, legacy.Deprecated,
				"`nb remote sync %s` must carry a deprecation notice", verb)
			assert.Equal(t, 1, strings.Count(legacy.Deprecated, "\n")+1,
				"the deprecation notice must be a single line")
			assert.Contains(t, legacy.Deprecated, "nb sync "+verb,
				"the notice must name the replacement spelling")
			assert.True(t, legacy.Runnable(),
				"a deprecated command must still run — nothing is removed")
		})
	}
}

// TestBothSpellingsBehaveIdentically: the legacy and canonical spellings are
// separate cobra instances of the same command, so their contracts — argument
// validation and flag surface — must match exactly.
func TestBothSpellingsBehaveIdentically(t *testing.T) {
	root := syncTestTree(t)

	for _, verb := range documentSyncVerbs {
		t.Run(verb, func(t *testing.T) {
			canonical := findCmd(t, root, "sync", verb)
			legacy := findCmd(t, root, "remote", "sync", verb)

			assert.Equal(t, canonical.Use, legacy.Use)
			assert.Equal(t, canonical.Short, legacy.Short)
			assert.Equal(t, flagSurface(canonical), flagSurface(legacy),
				"flag surfaces diverged between `nb sync %s` and `nb remote sync %s`", verb, verb)

			// Argument validation must agree, including on the arity errors
			// that make a typo'd invocation fail the same way in both places.
			for _, args := range [][]string{{}, {"one"}, {"one", "two"}} {
				canonicalErr := validateArgs(canonical, args)
				legacyErr := validateArgs(legacy, args)
				assert.Equal(t, canonicalErr, legacyErr,
					"arg validation for %v diverged: canonical=%v legacy=%v", args, canonicalErr, legacyErr)
			}
		})
	}
}

// flagSurface renders a command's flags as a comparable, order-independent
// summary of name, shorthand, type and default.
func flagSurface(cmd *cobra.Command) []string {
	var out []string
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		out = append(out, strings.Join([]string{f.Name, f.Shorthand, f.Value.Type(), f.DefValue}, "|"))
	})
	sort.Strings(out)
	return out
}

// validateArgs runs a command's Args validator, normalizing to a string so
// results compare cleanly. The command's own path is substituted out: cobra
// quotes it into arity errors, and each spelling correctly names itself — that
// is presentation, not a difference in what the command accepts.
func validateArgs(cmd *cobra.Command, args []string) string {
	if cmd.Args == nil {
		return "<nil validator>"
	}
	err := cmd.Args(cmd, args)
	if err == nil {
		return ""
	}
	return strings.ReplaceAll(err.Error(), cmd.CommandPath(), "<cmd>")
}
