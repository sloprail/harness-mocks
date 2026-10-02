package e2e

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT017_63_DisabledBackgroundTasksRunInTheForeground: with
// CLAUDE_CODE_DISABLE_BACKGROUND_TASKS=1 a Bash call that asks for
// run_in_background is not run in the background: it runs in the foreground,
// so its PostToolUse tool_response carries the command's own output and no
// backgroundTaskId, the agent gets that output rather than a receipt, and no
// task is started. Without the variable the same call is a background task.
// The docs name the variable and say it turns the background task
// functionality off; the foreground reading is the minimal literal one.
// sr:proves background-bash/claude
func TestT017_63_DisabledBackgroundTasksRunInTheForeground(t *testing.T) {
	for _, tc := range []struct {
		name       string
		env        []string
		background bool
	}{
		{"enabled", nil, true},
		{"disabled", []string{"CLAUDE_CODE_DISABLE_BACKGROUND_TASKS=1"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			cfg := filepath.Join(dir, "config")
			log := filepath.Join(dir, "payloads.log")
			settings(t, dir, map[string]string{"PostToolUse": payloadLogger(t, dir, "log.sh", log, "")})
			root := script(t, dir, "root", toolUse("dis", "Bash", `{"command":"echo FGOUT","description":"say it","run_in_background":true}`))
			out, code := runInDir(t, dir, tc.env, "--script", root, "--session-id", "dis-1", "--project-dir", dir, "--config-dir", cfg, "-p", "go")
			require.Equal(t, 0, code, out)

			var resp map[string]any
			for _, p := range payloads(t, log) {
				if id, _ := p["tool_use_id"].(string); p["tool_name"] == "Bash" && strings.HasPrefix(id, "dis") {
					resp, _ = p["tool_response"].(map[string]any)
				}
			}
			require.NotNil(t, resp, "no PostToolUse for the Bash call")
			frames := toolResultFrames(out)
			require.NotEmpty(t, frames)
			started := strings.Contains(out, `"subtype":"task_started"`)
			if tc.background {
				assert.NotEmpty(t, resp["backgroundTaskId"])
				assert.True(t, started, "a background task is started")
				return
			}
			assert.NotContains(t, resp, "backgroundTaskId")
			assert.Equal(t, "FGOUT", strings.TrimSpace(resp["stdout"].(string)))
			assert.Equal(t, "FGOUT", strings.TrimSpace(frames[0]["content"].(string)), "the agent gets the output, not a receipt")
			assert.False(t, started, "no task is started")
		})
	}
}

// TestT017_64_DisabledBackgroundTasksRunAnAgentInTheForeground: with the same
// variable an Agent call that asks for run_in_background runs the sub-agent in
// the foreground: the agent gets the sub-agent's final reply, not an
// async_launched receipt.
// sr:proves background-agent/claude
func TestT017_64_DisabledBackgroundTasksRunAnAgentInTheForeground(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	sub := write(t, filepath.Join(dir, "sub.sh"), `#!/bin/sh
echo '{"type":"assistant","message":{"role":"assistant","stop_reason":"end_turn","content":[{"type":"text","text":"AGENT-REPLY-6401"}]}}'
echo '{"type":"result","subtype":"success","result":"AGENT-REPLY-6401"}'
`, 0o755)
	sc := script(t, dir, "s", toolUse("ag1", "Agent", `{"prompt":"go","description":"fg agent","script":"`+sub+`","run_in_background":true}`))
	out, code := runInDir(t, dir, []string{"CLAUDE_CODE_DISABLE_BACKGROUND_TASKS=1"}, "--script", sc, "--session-id", "dis-2",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	assert.NotContains(t, out, "async_launched")
	assert.Contains(t, out, `"is_backgrounded":false`, "a foreground sub-agent's task frame")
	assert.NotContains(t, out, `"is_backgrounded":true`)
	assert.Contains(t, out, "AGENT-REPLY-6401")
}
