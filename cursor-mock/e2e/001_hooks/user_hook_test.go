package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAUserHookRunsFromTheProjectRootAndReadsItsPayload: the mock loads the user's
// ~/.cursor/hooks.json beside the project's and runs its hooks from the project root, the declared
// deviation (the docs say a user hook runs from ~/.cursor, which no recording shows): a user hook
// runs, from the project root, with its event's payload as JSON on stdin.
// sr:proves hook-command-handler/cursor
func TestAUserHookRunsFromTheProjectRootAndReadsItsPayload(t *testing.T) {
	ws, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	home, scratch := t.TempDir(), t.TempDir()
	logPath := filepath.Join(scratch, "log")
	hook := filepath.Join(scratch, "user.sh")
	require.NoError(t, os.WriteFile(hook, []byte("#!/bin/sh\ncat > \"$HOOK_LOG.in\"\npwd -P > \"$HOOK_LOG.pwd\"\n"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".cursor"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".cursor", "hooks.json"),
		[]byte(`{"version":1,"hooks":{"afterShellExecution":[{"command":`+jsonString(hook)+`}]}}`), 0o644))

	call := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu_0","name":"Bash","input":{"command":"echo hi"}}]}}`
	require.NoError(t, os.WriteFile(filepath.Join(scratch, "0.json"), []byte(call+"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(scratch, "end.json"), []byte(`{"type":"result","subtype":"success","result":"DONE"}`+"\n"), 0o644))
	script := filepath.Join(scratch, "scenario.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nn=$(grep -c '\"type\":\"tool_use\"' \"$A10N_MOCK_SESSION_FILE\" 2>/dev/null)\nn=${n:-0}\nf=\""+scratch+"/$n.json\"\n[ -f \"$f\" ] || f=\""+scratch+"/end.json\"\ncat \"$f\"\n"), 0o755))

	cmd := exec.Command(binary, "-p", "--force", "--output-format", "stream-json", "--script", script, "go")
	cmd.Dir, cmd.Env = ws, []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "HOOK_LOG=" + logPath, "PWD=" + ws}
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))

	pwd, err := os.ReadFile(logPath + ".pwd")
	require.NoError(t, err, "the user's hook never ran")
	require.Equal(t, ws, strings.TrimSpace(string(pwd)), "the mock runs a user hook from the project root")
	in, err := os.ReadFile(logPath + ".in")
	require.NoError(t, err)
	require.Contains(t, string(in), `"hook_event_name":"afterShellExecution"`, "the payload is on its stdin")
	require.Contains(t, string(in), `"command":"echo hi"`)
}
