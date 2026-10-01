package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The recorded run runs/pretool-refusal: preToolUse and beforeShellExecution
// hooks refuse commands by JSON and by exit status, a second preToolUse hook
// allows everything, and the turn goes on after every refusal.

// TestADenyInPreToolUseOutputRefusesTheCall: recorded, a preToolUse hook that
// prints {"permission":"deny",...} refuses the call: it does not run, no
// beforeShellExecution fires, and the failure hook reports permission_denied
// with the hook's user_message.
// sr:proves pretooluse-refusal/cursor
func TestADenyInPreToolUseOutputRefusesTheCall(t *testing.T) {
	got, want := replay(t, "pretool-refusal")
	conforms(t, got, want)

	require.Equal(t, "preToolUse postToolUseFailure", joined(eventsOf(got, "echo DENYME")))
	msg, kind, _ := failureOf(got, "echo DENYME")
	require.Equal(t, "USER-DENY-MSG", msg)
	require.Equal(t, "permission_denied", kind)
	require.Equal(t, "tool_call/completed/shellToolCall/rejected", got.frames[1], "the agent's result is a rejection")
}

// TestAnExit2InPreToolUseRefusesTheCall: recorded, a preToolUse hook exiting 2
// refuses the call the same way, with its stderr as the message.
// sr:proves pretooluse-refusal/cursor
// sr:proves hook-exit-code-semantics/cursor
func TestAnExit2InPreToolUseRefusesTheCall(t *testing.T) {
	got, want := replay(t, "pretool-refusal")
	conforms(t, got, want)

	require.Equal(t, "preToolUse postToolUseFailure", joined(eventsOf(got, "echo EXIT2PRE")))
	msg, _, _ := failureOf(got, "echo EXIT2PRE")
	require.Equal(t, "Hook blocked with message: PRE-BLOCK-MSG", msg)
}

// TestBeforeShellExecutionRefusesAShellCommandByExitStatusOrJSON: recorded, the
// shell-specific hook refuses a command too, by exit 2 or by a JSON deny,
// reporting it as "Command execution was blocked by a hook: ...".
// sr:proves pretooluse-refusal/cursor
func TestBeforeShellExecutionRefusesAShellCommandByExitStatusOrJSON(t *testing.T) {
	got, want := replay(t, "pretool-refusal")
	conforms(t, got, want)

	for cmd, why := range map[string]string{
		"echo EXIT2SHELL": "Hook blocked with message: SHELL-BLOCK-MSG",
		"echo DENYSHELL":  "SHELL-USER-DENY-MSG",
	} {
		require.Equal(t, "preToolUse beforeShellExecution postToolUseFailure", joined(eventsOf(got, cmd)), cmd)
		msg, kind, _ := failureOf(got, cmd)
		require.Contains(t, msg, "Command execution was blocked by a hook: "+why, cmd)
		require.Equal(t, "permission_denied", kind, cmd)
	}
}

// TestADenyWinsOverAnotherHooksAllow: recorded, when one preToolUse hook denies
// and another allows the same call, the call is refused, and both hooks ran.
// sr:proves pretooluse-refusal/cursor
func TestADenyWinsOverAnotherHooksAllow(t *testing.T) {
	got, want := replay(t, "pretool-refusal")
	conforms(t, got, want)

	require.Equal(t, "preToolUse postToolUseFailure", joined(eventsOf(got, "echo DENYWINS")))
	require.Contains(t, got.results, "allow:preToolUse:echo DENYWINS", "the allowing hook ran too")
}

// TestTheTurnGoesOnAfterARefusal: recorded, the call after the refusals runs.
// sr:proves pretooluse-refusal/cursor
func TestTheTurnGoesOnAfterARefusal(t *testing.T) {
	got, want := replay(t, "pretool-refusal")
	conforms(t, got, want)

	require.Equal(t, "preToolUse beforeShellExecution afterShellExecution postToolUse", joined(eventsOf(got, "echo FINE")))
}
