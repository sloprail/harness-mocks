package e2e

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPluginHooksRunAlongsideTheProjectsOwn: recorded (runs/plugin-hooks: the
// workspace holds two plugin directories, p1 and p2, each with a
// beforeShellExecution hook logging its name, and the project has one too;
// cursor-agent is started with --plugin-dir for p1 only), the hooks of p1 ran
// with the project's, and p2's did not.
// sr:proves plugin-hooks/cursor
func TestPluginHooksRunAlongsideTheProjectsOwn(t *testing.T) {
	got, want := replayWith(t, "plugin-hooks", "--plugin-dir", "plugins/p1")
	conforms(t, got, want)

	for name, o := range map[string]observed{"recorded": want, "mock": got} {
		var ran []string
		for _, r := range o.results {
			ran = append(ran, strings.Split(r, ":")[1])
		}
		require.Equal(t, []string{"p1", "project"}, ran, name+": p1's hook and the project's, not p2's")
	}
}
