package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

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

const shellCall = `{"type":"tool_call","subtype":"started","call_id":"c","tool_call":{"shellToolCall":{"args":{"command":"echo hi"}}}}`

// TestTheScriptRunsOncePerTurnAToolCallEndsTheTurnAndAResultEndsTheRun: the
// script plays the agent; each tool_call started frame ends its turn, the mock
// runs the tool and runs the script again, and the result ends the run.
// sr:proves turn-loop
func TestTheScriptRunsOncePerTurnAToolCallEndsTheTurnAndAResultEndsTheRun(t *testing.T) {
	out, _, code := run(t, `#!/bin/sh
if grep -q tool_use "$A10N_MOCK_SESSION_FILE"; then
  printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"result":"DONE"}'
else
  printf '%s\n' '`+shellCall+`'
fi
`, "go")
	require.Equal(t, 0, code, out)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Len(t, lines, 3, out)
	require.Contains(t, lines[0], `"subtype":"started"`)
	require.Contains(t, lines[1], `"subtype":"completed"`)
	require.Contains(t, lines[1], `"stdout":"hi\n"`)
	require.Contains(t, lines[2], `"type":"result"`, "the result is the last line of the stream")
}

// TestThePromptReachesTheScriptUnchanged: the script receives the user's
// prompt, unchanged, in A10N_MOCK_PROMPT.
// sr:proves scenario-prompt-env
func TestThePromptReachesTheScriptUnchanged(t *testing.T) {
	out, _, code := run(t, "#!/bin/sh\nprintf '{\"type\":\"result\",\"subtype\":\"success\",\"result\":\"%s\"}\\n' \"$A10N_MOCK_PROMPT\"\n", "fix", "the  bug")
	require.Equal(t, 0, code, out)
	require.Contains(t, out, `"result":"fix the  bug"`)
}

// TestTheScriptCanReadTheSessionSoFar: the transcript so far is readable
// through A10N_MOCK_SESSION_FILE: the user's prompt first.
// sr:proves session-file-env
func TestTheScriptCanReadTheSessionSoFar(t *testing.T) {
	out, _, code := run(t, "#!/bin/sh\nhead -c 60 \"$A10N_MOCK_SESSION_FILE\" | grep -q user_query && printf '%s\\n' '{\"type\":\"result\",\"subtype\":\"success\",\"result\":\"saw-prompt\"}'\n", "hello")
	require.Equal(t, 0, code, out)
	require.Contains(t, out, "saw-prompt")
}

// TestFiveIdenticalToolCallsInARowAbortTheRun: a script that never advances
// aborts the run with an error on the 5th identical call.
// sr:proves loop-guard
func TestFiveIdenticalToolCallsInARowAbortTheRun(t *testing.T) {
	out, stderr, code := run(t, "#!/bin/sh\nprintf '%s\\n' '"+shellCall+"'\n", "go")
	require.Equal(t, 1, code)
	require.Contains(t, stderr, "5 times in a row")
	require.Equal(t, 5, strings.Count(out, `"subtype":"completed"`), "the 5th identical call stops the run")
}
