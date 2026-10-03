package e2e

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The recorded run runs/hooks-all-matching-run-same-hook-two-sources: the same
// afterShellExecution command in the user's ~/.cursor/hooks.json and in the
// project's .cursor/hooks.json, and one shell call.

// TestTheSameHookInTheUserAndTheProjectSourceRunsOncePerSource: recorded, the
// command ran twice for the one call, once per source; Cursor does not run a
// hook configured in several sources once. The mock reads only the project
// source (a declared deviation), so replaying the project's file it runs the
// hook for the call once, the project's one: the recorded second run is the
// user source's.
// sr:proves hooks-all-matching-run/cursor
func TestTheSameHookInTheUserAndTheProjectSourceRunsOncePerSource(t *testing.T) {
	_, want, _, _ := recording(t, "hooks-all-matching-run-same-hook-two-sources")
	require.Len(t, want.hooks, 2, "the hook ran once per source")
	assert.Equal(t, want.hooks[0], want.hooks[1], "the same event twice")
	assert.Equal(t, "afterShellExecution", want.hooks[0]["hook_event_name"])
	assert.Equal(t, "echo TWICE-OR-ONCE", want.hooks[0]["command"])

	got, _ := replay(t, "hooks-all-matching-run-same-hook-two-sources")
	require.Len(t, got.hooks, 1, "the project's hook ran for the call")
	assert.Equal(t, want.hooks[0], got.hooks[0])
}
