package e2e

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bgSubagentScript is a sub-agent scenario that starts a background command
// (sleep 2, then touch marker) and then, once the receipt is in its transcript,
// ends by finish (a result frame line, or a failing exit).
func bgSubagentScript(marker, finish string) string {
	return `#!/bin/sh
if grep -q 'Command running in background' "$A10N_MOCK_SESSION_FILE" 2>/dev/null; then
  ` + finish + `
fi
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_bg","name":"Bash","input":{"command":"sleep 2; touch ` + marker + `","description":"bgsleep","run_in_background":true}}]}}'
`
}

// TestT009_22_FailedForegroundSubagentEndsItsBackgroundCommand: a foreground
// sub-agent's background command ends with the sub-agent's run whether it
// finished or failed (tools-reference#background-commands); a sub-agent whose
// scenario exits non-zero has failed.
// sr:proves foreground-subagent-bash-ends-with-response/claude
func TestT009_22_FailedForegroundSubagentEndsItsBackgroundCommand(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "survived")
	sub := writeScript(t, dir, "sub.sh", bgSubagentScript(marker, "exit 3"))
	out, code := runInDir(t, dir, nil,
		"--script", writeScript(t, dir, "orch.sh", orchestratorScript("Agent", sub)),
		"--session-id", "fg-fail", "--project-dir", dir, "--config-dir", filepath.Join(dir, "config"), "-p", "go")
	require.Equal(t, 0, code, out)
	assert.Contains(t, out, `"status":"stopped"`, "the command is stopped when the sub-agent has failed")
	time.Sleep(3 * time.Second)
	_, err := os.Stat(marker)
	assert.True(t, os.IsNotExist(err), "the command was terminated, not left to finish")
}

// TestT009_23_BackgroundSubagentsCommandOutlivesItsResponse: a background
// sub-agent's background command keeps running after the sub-agent's final
// response, until it exits (tools-reference#background-commands).
// sr:proves foreground-subagent-bash-ends-with-response/claude
func TestT009_23_BackgroundSubagentsCommandOutlivesItsResponse(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "survived")
	sub := writeScript(t, dir, "sub.sh", bgSubagentScript(marker,
		`printf '%s\n' '{"type":"result","subtype":"success","result":"sub done","is_error":false}'; exit 0`))
	// The main thread keeps busy with a foreground command of its own while the
	// sub-agent responds and its command runs on.
	orch := writeScript(t, dir, "orch.sh", `#!/bin/sh
F="$A10N_MOCK_SESSION_FILE"
if grep -q 'toolu_sl' "$F" 2>/dev/null; then
  printf '%s\n' '{"type":"result","subtype":"success","result":"orchestrated","is_error":false}'
  exit 0
fi
if grep -q 'toolu_ag' "$F" 2>/dev/null; then
  printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_sl","name":"Bash","input":{"command":"sleep 3"}}]}}'
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_ag","name":"Agent","input":{"description":"bg","prompt":"go","script":"`+sub+`","run_in_background":true}}]}}'
`)
	out, code := runInDir(t, dir, nil, "--script", orch,
		"--session-id", "bg-bg", "--project-dir", dir, "--config-dir", filepath.Join(dir, "config"), "-p", "go")
	require.Equal(t, 0, code, out)
	_, err := os.Stat(marker)
	assert.NoError(t, err, "the command ran to its end after the sub-agent had responded")
}
