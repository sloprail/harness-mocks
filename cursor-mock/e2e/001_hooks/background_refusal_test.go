package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAPreToolUseHookRefusesATaskCallAndNoSubAgentRuns: the preToolUse hook
// fires for a Task call (recorded: runs/print-waits-for-background-agents), and
// one that exits 2 refuses it like any call: the call ends in an error result
// carrying the hook's message, no sub-agent runs, and so no session is left
// waiting for one.
// sr:proves pretooluse-refusal/cursor
func TestAPreToolUseHookRefusesATaskCallAndNoSubAgentRuns(t *testing.T) {
	ws, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	scratch := t.TempDir()
	ran := filepath.Join(scratch, "sub-ran")
	put := func(path, body string) {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o755))
	}
	put(filepath.Join(ws, ".cursor", "hooks.json"), `{"version":1,"hooks":{"preToolUse":[{"command":".cursor/hooks/deny.sh","matcher":"Task"}]}}`)
	put(filepath.Join(ws, ".cursor", "hooks", "deny.sh"), "#!/bin/sh\ncat >/dev/null\necho NO-TASKS >&2\nexit 2\n")
	sub := filepath.Join(scratch, "sub.sh")
	put(sub, "#!/bin/sh\ntouch "+ran+"\n")
	script := filepath.Join(scratch, "main.sh")
	put(script, `#!/bin/sh
if grep -q '"name":"Task"' "$A10N_MOCK_SESSION_FILE" 2>/dev/null; then
echo '{"type":"result","subtype":"success","result":"END"}'; else
echo '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Task","input":{"description":"d","prompt":"p","run_in_background":true,"script":"`+sub+`"}}]}}'; fi
`)
	cmd := exec.Command(binary, "-p", "--force", "--trust", "--output-format", "stream-json", "go")
	cmd.Dir, cmd.Env = ws, []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir(), "A10N_MOCK_SCRIPT=" + script}
	out, err := cmd.Output()
	require.NoError(t, err, "%s", out)

	kinds, frames := streamKinds(t, string(out))
	require.Equal(t, []string{"task/started", "task/completed", "result/success"}, kinds)
	res := frames[1]["tool_call"].(map[string]any)["taskToolCall"].(map[string]any)["result"].(map[string]any)
	require.Contains(t, res["error"].(map[string]any)["errorMessage"], "NO-TASKS")
	_, err = os.Stat(ran)
	require.True(t, os.IsNotExist(err), "the refused sub-agent never ran")
}
