package e2e

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A foreground sub-agent's tool call that a PreToolUse hook refuses is denied
// like the main agent's: the sub-agent is told the refusal as the tool's result
// (runs/all-hooks for the text), the tool does not run and no PostToolUse fires
// for it, and the sub-agent goes on to hand its report back, completed
// (docs: foreground sub-agents are asked for permissions as they arise; the mock
// runs under --dangerously-skip-permissions, so a hook's refusal is the denial).
// sr:proves foreground-subagent-result/claude
func TestT017_87_AForegroundSubagentsRefusedToolCallIsDenied(t *testing.T) {
	dir := t.TempDir()
	cfg, log := filepath.Join(dir, "config"), filepath.Join(dir, "payloads.log")
	deny := write(t, filepath.Join(dir, "deny.sh"), "#!/bin/sh\ncat >/dev/null\necho SUB-DENIED >&2\nexit 2\n", 0o755)
	compactSettings(t, dir, map[string][2]string{"PreToolUse": {"Bash", deny}, "PostToolUse": {"*", payloadLogger(t, dir, "post.sh", log, "")}})
	sub := callThenReply(t, dir, "sub", "REPORTED", toolUse("sb1", "Bash", `{"command":"touch ran-anyway"}`))
	orch := script(t, dir, "orch", toolUse("ag1", "Agent", `{"prompt":"go","description":"denied","run_in_background":false,"script":"`+sub+`"}`))
	out, code := runInDir(t, dir, nil, "--script", orch, "--session-id", "subdeny-1", "--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)

	files, err := filepath.Glob(filepath.Join(cfg, "projects", "*", "subdeny-1", "subagents", "agent-*.jsonl"))
	require.NoError(t, err)
	require.Len(t, files, 1)
	var told bool
	for _, r := range readRecs(t, files[0]) {
		told = told || strings.Contains(r.Raw, "PreToolUse:Bash hook error: [")
	}
	assert.True(t, told, "the sub-agent is told the refusal as its tool's result")
	assert.NoFileExists(t, filepath.Join(dir, "ran-anyway"), "the refused tool did not run")
	posts := payloads(t, log)
	require.Len(t, posts, 1, "only the Agent call's PostToolUse")
	assert.Equal(t, "Agent", posts[0]["tool_name"])
	assert.Contains(t, out, "REPORTED", "the sub-agent still hands its report back")
}
