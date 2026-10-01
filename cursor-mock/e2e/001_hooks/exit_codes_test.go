package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The recorded run runs/hook-exit-codes: one hook script on every event exits
// with the status each command asks for (2, 1, 3), prints text that is not
// JSON, or exits 2 from afterShellExecution.

// TestExit2OnBeforeShellExecutionBlocksTheCommandWithStderrAsItsMessage:
// recorded, a beforeShellExecution hook exiting 2 stops the command: no
// afterShellExecution, no postToolUse, only the failure hook (permission_denied)
// whose message is the hook's stderr.
// sr:proves hook-exit-code-semantics/cursor
func TestExit2OnBeforeShellExecutionBlocksTheCommandWithStderrAsItsMessage(t *testing.T) {
	got, want := replay(t, "hook-exit-codes")
	conforms(t, got, want)

	require.Equal(t, "preToolUse beforeShellExecution postToolUseFailure", joined(eventsOf(got, "echo EXIT2ME")))
	msg, kind, ok := failureOf(got, "echo EXIT2ME")
	require.True(t, ok)
	require.Equal(t, "permission_denied", kind)
	require.Contains(t, msg, "Hook blocked with message: SHELL-BLOCK-MSG")
}

// TestAnyOtherExitStatusFailsOpen: recorded, a hook exiting 1 or 3 does not
// stop the command: it runs and its after-hooks fire.
// sr:proves hook-exit-code-semantics/cursor
func TestAnyOtherExitStatusFailsOpen(t *testing.T) {
	got, want := replay(t, "hook-exit-codes")
	conforms(t, got, want)

	for _, cmd := range []string{"echo EXIT1ME", "echo EXIT3ME"} {
		require.Equal(t, "preToolUse beforeShellExecution afterShellExecution postToolUse", joined(eventsOf(got, cmd)), cmd)
		_, _, failed := failureOf(got, cmd)
		require.False(t, failed, cmd)
	}
}

// TestExit2FromAnEventThatHasNothingToBlockChangesNothing: recorded, an
// afterShellExecution hook exiting 2 leaves the call as it was: postToolUse
// still fires.
// sr:proves hook-exit-code-semantics/cursor
func TestExit2FromAnEventThatHasNothingToBlockChangesNothing(t *testing.T) {
	got, want := replay(t, "hook-exit-codes")
	conforms(t, got, want)

	require.Equal(t, "preToolUse beforeShellExecution afterShellExecution postToolUse", joined(eventsOf(got, "echo AFTER2ME")))
}

// TestAHookConfiguredFailClosedBlocksOnAnyFailure: recorded (runs/hook-fail-closed),
// with failClosed true a beforeShellExecution hook exiting 1 or 3, or printing
// nothing, blocks the command, and says it failed closed.
// sr:proves hook-exit-code-semantics/cursor
func TestAHookConfiguredFailClosedBlocksOnAnyFailure(t *testing.T) {
	got, want := replay(t, "hook-fail-closed")
	conforms(t, got, want)

	for cmd, why := range map[string]string{
		"echo EXIT1ME": `failed with exit code 1: SHELL-EXIT1-MSG`,
		"echo EXIT3ME": `failed with exit code 3: SHELL-EXIT3-MSG`,
		"echo FINE":    `returned no output.`,
	} {
		require.Equal(t, "preToolUse beforeShellExecution postToolUseFailure", joined(eventsOf(got, cmd)), cmd)
		msg, kind, _ := failureOf(got, cmd)
		require.Contains(t, msg, "configured to fail closed", cmd)
		require.Contains(t, msg, why, cmd)
		require.Equal(t, "permission_denied", kind, cmd)
	}
}

// TestOutputThatIsNotJSONBlocksAPermissionHook: recorded, a beforeShellExecution
// hook that exits 0 with text that is not JSON blocks the command for safety;
// one that prints nothing allows it.
// sr:proves hook-exit-code-semantics/cursor
func TestOutputThatIsNotJSONBlocksAPermissionHook(t *testing.T) {
	got, want := replay(t, "hook-exit-codes")
	conforms(t, got, want)

	msg, kind, ok := failureOf(got, "echo BADJSON")
	require.True(t, ok)
	require.Equal(t, "permission_denied", kind)
	require.Contains(t, msg, `returned invalid JSON. The command was blocked for safety.`)
	require.Equal(t, "preToolUse beforeShellExecution afterShellExecution postToolUse", joined(eventsOf(got, "echo FINE")), "a hook with no output allows")
}

// TestAHookCommandThatCannotRunFailsOpenUnlessItFailsClosed: not recorded; the
// docs' failClosed row says a hook that crashes blocks the action when
// failClosed is true, and that other failures fail open. A hook command that
// is not there makes the shell exit 127, the case the mock can run: the
// command goes on by default, and is blocked with failClosed.
// sr:proves hook-exit-code-semantics/cursor
func TestAHookCommandThatCannotRunFailsOpenUnlessItFailsClosed(t *testing.T) {
	open := runCustom(t, `{"version":1,"hooks":{"beforeShellExecution":[{"command":".cursor/hooks/missing.sh"}]}}`, nil, "echo RAN")
	require.Contains(t, open.stdout, "RAN", "the command ran")
	require.NotContains(t, open.stdout, "blocked")

	closed := runCustom(t, `{"version":1,"hooks":{"beforeShellExecution":[{"command":".cursor/hooks/missing.sh","failClosed":true}]}}`, nil, "echo RAN")
	require.Contains(t, closed.stdout, "configured to fail closed")
	require.Contains(t, closed.stdout, "failed with exit code 127")
}
