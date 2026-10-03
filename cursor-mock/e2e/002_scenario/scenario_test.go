package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The scenario protocol is every mock's, unchanged: the script prints
// stream-json lines of Claude Code's shape (an assistant line whose tool_use
// block is the agent's tool call, a result line to end the run), and the mock
// prints Cursor's own frames for it.

// run plays a scenario script against the mock in a fresh workspace and
// returns its stdout, stderr and exit status.
func run(t *testing.T, script string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "scenario.sh")
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	cmd := exec.Command(binary, append([]string{"-p", "--output-format", "stream-json", "--script", path}, args...)...)
	cmd.Dir, cmd.Env = dir, []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir()}
	var out, errb strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
	}
	return out.String(), errb.String(), code
}

const (
	bashCall = `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu_1","name":"Bash","input":{"command":"echo hi"}}]}}`
	done     = `{"type":"result","subtype":"success","is_error":false,"result":"DONE"}`
)

// TestTheScriptRunsOncePerTurnAToolCallEndsTheTurnAndAResultEndsTheRun: the
// script plays the agent; each tool call ends its turn, the mock runs the tool
// and runs the script again, and the result ends the run.
// sr:proves turn-loop
// sr:proves noninteractive-run/cursor
func TestTheScriptRunsOncePerTurnAToolCallEndsTheTurnAndAResultEndsTheRun(t *testing.T) {
	out, _, code := run(t, `#!/bin/sh
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"on it"}]}}'
if grep -q tool_use "$A10N_MOCK_SESSION_FILE"; then
  printf '%s\n' '`+done+`'
else
  printf '%s\n' '`+bashCall+`'
fi
`, "go")
	require.Equal(t, 0, code, out)
	var types []string
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		var f struct{ Type string }
		require.NoError(t, json.Unmarshal([]byte(l), &f))
		types = append(types, f.Type)
	}
	// init, the user's prompt, then per turn the agent's words, and for the
	// first the call (started, completed); the result is the last line.
	require.Equal(t, []string{"system", "user", "assistant", "tool_call", "tool_call", "assistant", "result"}, types, out)
	require.Contains(t, out, `"stdout":"hi\n"`)
	require.Contains(t, out, `"result":"on iton it"`)
}

// TestThePromptReachesTheScriptUnchanged: the script receives the user's
// prompt, unchanged, in A10N_MOCK_PROMPT.
// sr:proves scenario-prompt-env
func TestThePromptReachesTheScriptUnchanged(t *testing.T) {
	// The script says the prompt it was given before it ends the run, and
	// Cursor's result frame carries what the agent said, so the assertion is on
	// that result, not on the user frame the mock echoes from the command line.
	out, _, code := run(t, "#!/bin/sh\nprintf '%s\\n' '{\"type\":\"assistant\",\"message\":{\"content\":[{\"type\":\"text\",\"text\":\"got:'\"$A10N_MOCK_PROMPT\"'\"}]}}'\n"+
		"printf '%s\\n' '"+done+"'\n", "fix", "the  bug")
	require.Equal(t, 0, code, out)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	var last struct{ Type, Result string }
	require.NoError(t, json.Unmarshal([]byte(lines[len(lines)-1]), &last))
	require.Equal(t, "result", last.Type, out)
	require.Equal(t, "got:fix the  bug", last.Result)
}

// TestTheScriptCanReadTheSessionSoFar: the transcript so far is readable
// through A10N_MOCK_SESSION_FILE: the user's prompt first.
// sr:proves session-file-env
func TestTheScriptCanReadTheSessionSoFar(t *testing.T) {
	out, _, code := run(t, "#!/bin/sh\nif grep -q user_query \"$A10N_MOCK_SESSION_FILE\"; then printf '%s\\n' '{\"type\":\"assistant\",\"message\":{\"content\":[{\"type\":\"text\",\"text\":\"saw-prompt\"}]}}'; fi\n"+
		"printf '%s\\n' '"+done+"'\n", "hello")
	require.Equal(t, 0, code, out)
	require.Contains(t, out, "saw-prompt")
}

// TestFiveIdenticalToolCallsInARowAbortTheRun: a script that never advances
// aborts the run with an error at the 5th identical call.
// sr:proves loop-guard
func TestFiveIdenticalToolCallsInARowAbortTheRun(t *testing.T) {
	out, stderr, code := run(t, "#!/bin/sh\nprintf '%s\\n' '"+bashCall+"'\n", "go")
	require.Equal(t, 1, code)
	require.Contains(t, stderr, "5 turns in a row")
	require.NotContains(t, out, `"type":"result"`, "an aborted run reports no result")
}

// TestStreamPartialOutputIsAcceptedAndChangesNothing: --stream-partial-output
// is accepted and ignored: the stream has the same frame types, and a single
// result frame, with or without it (no partial assistant deltas are modeled).
// sr:proves noninteractive-run/cursor
func TestStreamPartialOutputIsAcceptedAndChangesNothing(t *testing.T) {
	script := "#!/bin/sh\nprintf '%s\\n' '{\"type\":\"assistant\",\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"text\",\"text\":\"hi\"}]}}'\nprintf '%s\\n' '" + done + "'\n"
	types := func(out string) (ts []string) {
		for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
			var f struct{ Type string }
			require.NoError(t, json.Unmarshal([]byte(l), &f))
			ts = append(ts, f.Type)
		}
		return ts
	}
	plain, _, code := run(t, script, "go")
	require.Equal(t, 0, code, plain)
	partial, _, code := run(t, script, "--stream-partial-output", "go")
	require.Equal(t, 0, code, partial)
	require.Equal(t, types(plain), types(partial))
	require.Equal(t, 1, strings.Count(partial, `"type":"result"`))
}

// TestAnOutputFormatOtherThanStreamJSONIsRefused: the mock models stream-json
// only (the format the cursor docs describe for real-time progress); any other
// --output-format ends the run with exit status 1, a message naming the format
// on stderr and nothing on stdout.
// sr:proves noninteractive-run/cursor
func TestAnOutputFormatOtherThanStreamJSONIsRefused(t *testing.T) {
	for _, format := range []string{"text", "json"} {
		out, errOut, code := run(t, "#!/bin/sh\ntouch ran\nprintf '%s\\n' '"+done+"'\n", "--output-format", format, "go")
		require.Equal(t, 1, code, format)
		require.Empty(t, out, format)
		require.Contains(t, errOut, `output format "`+format+`" is not modeled`, format)
	}
}
