package e2e

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// shellOutputs are what each shell command printed, as afterShellExecution saw
// it, by command.
func shellOutputs(o observed) map[string]string {
	out := map[string]string{}
	for _, h := range o.hooks {
		if h["hook_event_name"] == "afterShellExecution" {
			out[h["command"].(string)] = h["output"].(string)
		}
	}
	return out
}

// The recorded runs runs/subprocess-session-env and runs/nested-session-env:
// a shell command lists the CURSOR* variables it sees and prints the values of
// the ones that identify the harness and the session.

// TestAShellCommandSeesTheHarnessTheSessionAndHowItWasStarted: recorded, the
// Shell tool's children see CURSOR_AGENT=1, CURSOR_CONVERSATION_ID (the
// session's id: the same one every hook payload carries) and
// CURSOR_INVOKED_AS=cursor-agent.
// sr:proves subprocess-session-env/cursor
func TestAShellCommandSeesTheHarnessTheSessionAndHowItWasStarted(t *testing.T) {
	got, want := replay(t, "subprocess-session-env")
	conforms(t, got, want)

	var values string
	for cmd, out := range shellOutputs(got) {
		if strings.Contains(cmd, "CURSOR_INVOKED_AS|") {
			values = out
		}
	}
	require.Equal(t, "CURSOR_AGENT=1\nCURSOR_CONVERSATION_ID=<SESSION_ID>\nCURSOR_INVOKED_AS=cursor-agent\n", values)
}

// TestInsideAnotherSessionTheSessionIdIsThisRuns: recorded, a run launched
// with another session's CURSOR_CONVERSATION_ID and CURSOR_INVOKED_AS in its
// environment hands its commands its own; CURSOR_AGENT, which the harness only
// defaults, passes through.
// sr:proves subprocess-session-env/cursor
func TestInsideAnotherSessionTheSessionIdIsThisRuns(t *testing.T) {
	got, want := replay(t, "nested-session-env")
	conforms(t, got, want)

	var values string
	for cmd, out := range shellOutputs(got) {
		if strings.Contains(cmd, "CURSOR_INVOKED_AS|") {
			values = out
		}
	}
	require.Contains(t, values, "CURSOR_CONVERSATION_ID=<SESSION_ID>\n", "this run's session, not decoy-conversation")
	require.Contains(t, values, "CURSOR_INVOKED_AS=cursor-agent\n")
	require.Contains(t, values, "CURSOR_AGENT=decoy-agent\n")
}
