package e2e

import (
	"os"
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

// TestAHookCommandSeesTheProjectTheVersionAndHowItWasStarted: recorded, a hook
// process sees CURSOR_PROJECT_DIR, CLAUDE_PROJECT_DIR (the workspace),
// CURSOR_VERSION and CURSOR_INVOKED_AS=cursor-agent, and CURSOR_TRANSCRIPT_PATH
// once the transcript is named in the payloads.
// sr:proves subprocess-session-env/cursor
func TestAHookCommandSeesTheProjectTheVersionAndHowItWasStarted(t *testing.T) {
	got, want := replay(t, "subprocess-session-env")
	conforms(t, got, want)
	require.Equal(t, want.envs, got.envs)
	require.NotEmpty(t, got.envs)
	first := got.envs[0]
	require.Equal(t, "<RUN>", first["CURSOR_PROJECT_DIR"])
	require.Equal(t, "<RUN>", first["CLAUDE_PROJECT_DIR"])
	require.Equal(t, "cursor-agent", first["CURSOR_INVOKED_AS"])

	c := runCustom(t, `{"version":1,"hooks":{"sessionStart":[{"command":"cat >/dev/null; echo \"[$CURSOR_TRANSCRIPT_PATH]\" > \"$HOOK_LOG.start\""}],"sessionEnd":[{"command":"cat >/dev/null; echo \"$CURSOR_TRANSCRIPT_PATH\" > \"$HOOK_LOG.end\""}]}}`, nil, "echo hi")
	start, _ := os.ReadFile(c.log + ".start")
	require.Equal(t, "[]", strings.TrimSpace(string(start)), "no transcript path yet at sessionStart")
	end, _ := os.ReadFile(c.log + ".end")
	path, _ := c.transcript(t)
	require.Equal(t, path, strings.TrimSpace(string(end)), "the transcript file, once the conversation has one")
}

// TestInsideAnotherSessionAHookKeepsWhatItInheritedButHowItWasStarted:
// recorded, over decoys the hook's CURSOR_INVOKED_AS is this run's, and its
// CURSOR_PROJECT_DIR, CURSOR_VERSION, CURSOR_CONVERSATION_ID and CURSOR_AGENT
// are the inherited ones.
// sr:proves subprocess-session-env/cursor
func TestInsideAnotherSessionAHookKeepsWhatItInheritedButHowItWasStarted(t *testing.T) {
	got, want := replay(t, "nested-session-env")
	conforms(t, got, want)
	require.Equal(t, want.envs, got.envs)
	e := got.envs[0]
	require.Equal(t, "cursor-agent", e["CURSOR_INVOKED_AS"])
	require.Equal(t, "decoy-project", e["CURSOR_PROJECT_DIR"])
	require.Equal(t, "decoy-version", e["CURSOR_VERSION"])
	require.Equal(t, "decoy-conversation", e["CURSOR_CONVERSATION_ID"])
	require.Equal(t, "decoy-agent", e["CURSOR_AGENT"])
}
