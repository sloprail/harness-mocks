package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The recorded run runs/hook-timeout-early-output: a beforeShellExecution
// hook with a 1 s timeout that prints its deny at once and then runs on for
// 5 s.

// TestWhatAHookPrintedBeforeItsTimeoutIsDiscardedAndTheCommandRuns: recorded,
// the deny the hook printed before it timed out is not acted on: the command
// runs, and the hook is killed before it finishes.
// sr:proves hook-timeout/cursor
func TestWhatAHookPrintedBeforeItsTimeoutIsDiscardedAndTheCommandRuns(t *testing.T) {
	got, want := replay(t, "hook-timeout-early-output")
	conforms(t, got, want)

	require.Contains(t, got.results, "ran:EARLYOPEN:<nil>:<nil>:<nil>:started")
	for _, r := range got.results {
		require.NotContains(t, r, ":finished", "the hook was killed before it finished")
	}
	var after []string
	for _, h := range got.hooks {
		if h["hook_event_name"] == "afterShellExecution" {
			after = append(after, h["command"].(string))
		}
		require.NotEqual(t, "postToolUseFailure", h["hook_event_name"], "the early deny blocked nothing")
	}
	require.Equal(t, []string{"echo EARLYOPEN"}, after, "the command ran")
}
