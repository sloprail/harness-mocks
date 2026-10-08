package e2e

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// The recorded run runs/multiroot-workspace: a shell command run with a second
// workspace root added on the command line (--add-dir ../second-root).

// TestEveryHookNamesOneWorkspaceRootEvenWithASecondRootAdded: recorded, a run
// that adds a second root with --add-dir still tells every hook one
// workspace root, the project's: the added directory does not show in the
// payloads' workspace_roots. The mock does the same.
// sr:proves hook-common-payload/cursor
func TestEveryHookNamesOneWorkspaceRootEvenWithASecondRootAdded(t *testing.T) {
	got, want := replayWith(t, "multiroot-workspace", "--add-dir", "../second-root")
	conforms(t, got, want)

	recorded := readJSONL(t, filepath.Join(newestSample(t, "multiroot-workspace"), "payloads.jsonl"))
	for name, o := range map[string]struct {
		payloads []map[string]any
		root     string
	}{"recorded": {recorded, "<RUN>"}, "mock": {got.raw, got.ws}} {
		require.NotEmpty(t, o.payloads, name)
		for _, p := range o.payloads {
			roots, _ := p["workspace_roots"].([]any)
			require.Equal(t, []any{o.root}, roots, name+": "+p["hook_event_name"].(string))
		}
	}
}
