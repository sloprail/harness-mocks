package e2e

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPluginHooksRunBeforeTheProjectsOwn: recorded (runs/plugin-hooks: the
// workspace holds two plugin directories, p1 and p2, each with a
// beforeShellExecution hook logging its name, and the project has one too;
// cursor-agent is started with --plugin-dir for p1 only), the hooks of p1 and
// the project's both ran, and p2's did not. An event's hooks run together, so
// the order they log in is the order they finish in, which the recording does
// not promise (its own sample is one such order): only which ran is asserted.
// sr:proves plugin-hooks/cursor
func TestPluginHooksRunBeforeTheProjectsOwn(t *testing.T) {
	got, want := replayWith(t, "plugin-hooks", "--plugin-dir", "plugins/p1")
	conforms(t, got, want)

	// the hooks of one event run at once (internal/hooks.RunAll), so the log
	// holds them in the order they finished: compare what ran, not that order
	for name, o := range map[string]observed{"recorded": want, "mock": got} {
		var ran []string
		for _, r := range o.results {
			ran = append(ran, strings.Split(r, ":")[1])
		}
		require.ElementsMatch(t, []string{"p1", "project"}, ran, name+": p1's hook and the project's, not p2's")
	}
}
