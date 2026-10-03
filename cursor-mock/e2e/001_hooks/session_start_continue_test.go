package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestASessionStartHookSayingContinueFalseDoesNotStopTheSession: recorded
// (runs/session-start-continue-false: a sessionStart hook that prints
// {"continue": false, "user_message": ...} and exits 0, with hooks configured
// for every event the run could fire, beforeSubmitPrompt and stop among them),
// cursor-agent's session goes on exactly as it does for a start hook that says
// nothing: the command runs, its hooks fire, the sessionEnd hook closes the run
// and the stream ends in a success result. The docs say the same ("Session
// creation is not blocked even when continue is false"). The recording shows
// print mode firing neither beforeSubmitPrompt nor stop, and the mock fires
// the same events.
// sr:proves session-start-hook/cursor
func TestASessionStartHookSayingContinueFalseDoesNotStopTheSession(t *testing.T) {
	got, want := replay(t, "session-start-continue-false")
	conforms(t, got, want)

	const whole = "sessionStart preToolUse beforeShellExecution afterShellExecution postToolUse sessionEnd"
	require.Equal(t, whole, joined(hookNames(want)), "the recorded session went on after the start hook said continue: false")
	require.Equal(t, whole, joined(hookNames(got)))
	for _, never := range []string{"beforeSubmitPrompt", "stop"} {
		require.NotContains(t, hookNames(want), never, "recorded: print mode fires no "+never+" hook")
		require.NotContains(t, hookNames(got), never)
	}
	require.Contains(t, want.results, "sessionStart:0", "the start hook exited 0 with its refusal in the output")
	require.Contains(t, got.results, "sessionStart:0")

	require.Equal(t, []string{
		"tool_call/started/shellToolCall/", "tool_call/completed/shellToolCall/success", "result/success",
	}, got.frames[len(got.frames)-3:], "the command ran and the run ended in a success result")
	require.Equal(t, want.frames, got.frames)
}
