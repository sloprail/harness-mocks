package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT009_19_ForegroundSubagentBackgroundCommandEndsWithItsResponse: a background
// command a foreground sub-agent starts is terminated when that sub-agent gives
// its final response, and its receipt says so (recorded:
// snapshots/runs/fg-subagent-bash): "it is terminated when you give your final
// response" with backgroundEndsWithFinalResponse true, a killed command's output
// file ending "[killed]", and a stopped notification frame.
// sr:proves foreground-subagent-bash-ends-with-response/claude
func TestT009_19_ForegroundSubagentBackgroundCommandEndsWithItsResponse(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "survived")
	tmp := filepath.Join(dir, "tmp")
	sub := writeScript(t, dir, "sub.sh", `#!/bin/sh
if grep -q 'Command running in background' "$A10N_MOCK_SESSION_FILE" 2>/dev/null; then
  printf '%s\n' '{"type":"result","subtype":"success","result":"sub done","is_error":false}'
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_bg","name":"Bash","input":{"command":"sleep 3; touch `+marker+`","description":"bgsleep","run_in_background":true}}]}}'
`)
	out, code := runInDir(t, dir, []string{"CLAUDE_CODE_TMPDIR=" + tmp},
		"--script", writeScript(t, dir, "orch.sh", orchestratorScript("Agent", sub)),
		"--session-id", "fg-bg", "--project-dir", dir, "--config-dir", filepath.Join(dir, "config"), "-p", "go")
	require.Equal(t, 0, code, out)

	assert.Contains(t, out, `"subtype":"task_started"`)
	assert.Contains(t, out, `"task_type":"local_bash"`)
	assert.Contains(t, out, `"status":"stopped"`, "the command is stopped when the sub-agent responds")

	sidechains, err := filepath.Glob(filepath.Join(dir, "config", "projects", "*", "fg-bg", "subagents", "agent-*.jsonl"))
	require.NoError(t, err)
	require.Len(t, sidechains, 1)
	raw, err := os.ReadFile(sidechains[0])
	require.NoError(t, err)
	assert.Contains(t, string(raw), "it is terminated when you give your final response")
	assert.Contains(t, string(raw), `"backgroundEndsWithFinalResponse":true`)

	outputs, err := filepath.Glob(filepath.Join(tmp, "claude-*", "*", "fg-bg", "tasks", "b*.output"))
	require.NoError(t, err)
	require.Len(t, outputs, 1)
	body, err := os.ReadFile(outputs[0])
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(string(body), "[killed]\n"), "output file: %q", body)

	time.Sleep(3500 * time.Millisecond)
	_, err = os.Stat(marker)
	assert.True(t, os.IsNotExist(err), "the command was terminated, not left to finish")
}
