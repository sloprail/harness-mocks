package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The recorded run runs/hook-matchers-task-read: preToolUse hooks matched on
// Task, Shell and Read, beforeReadFile hooks matched on Read and Shell, and
// postToolUse hooks matched on Task and Read, around a Task call (a sub-agent)
// and a Read.

// TestAMatcherIsTestedAgainstTaskAndBeforeReadFileTakesTheToolNameRead:
// recorded, a preToolUse hook whose matcher is Task runs for the Task call and
// the Shell one does not; a beforeReadFile hook whose matcher is Read runs for
// a read and the one matched on Shell does not; the postToolUse hook matched on
// Task does not run, for no postToolUse fires for a Task call. The mock runs the
// same hooks.
// sr:proves hook-matcher-filter/cursor
func TestAMatcherIsTestedAgainstTaskAndBeforeReadFileTakesTheToolNameRead(t *testing.T) {
	got, want := replay(t, "hook-matchers-task-read")
	conforms(t, got, want)

	for name, o := range map[string]observed{"recorded": want, "mock": got} {
		require.Contains(t, o.results, "ran:pre-Task:preToolUse:Task::<nil>", name)
		require.Contains(t, o.results, "ran:readfile-Read:beforeReadFile:::<nil>", name)
		for _, r := range o.results {
			require.NotContains(t, r, "pre-Shell", name)
			require.NotContains(t, r, "readfile-Shell", name)
			require.NotContains(t, r, "post-Task", name)
		}
	}
}
