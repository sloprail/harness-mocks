package e2e

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A SubagentStart hook cannot block: exit 2 is a non-blocking error, the
// foreground sub-agent still runs, SubagentStop fires and the Agent call hands
// back the sub-agent's report as completed (runs/hookerrors).
// sr:proves foreground-subagent-result/claude
func TestT017_85_SubagentStartExit2StillHandsBackTheReport(t *testing.T) {
	want := recordedAgentPost(t, "hookerrors", 0)["tool_response"].(map[string]any)

	dir := t.TempDir()
	cfg, log := filepath.Join(dir, "config"), filepath.Join(dir, "payloads.log")
	fail := write(t, filepath.Join(dir, "fail.sh"), "#!/bin/sh\ncat >/dev/null\necho start-refused >&2\nexit 2\n", 0o755)
	settings(t, dir, map[string]string{"SubagentStart": fail, "SubagentStop": payloadLogger(t, dir, "log.sh", log, ""),
		"PostToolUse": payloadLogger(t, dir, "post.sh", log, "")})
	sub := replyScript(t, dir, "sub", "HELPED")
	orch := script(t, dir, "orch", toolUse("ag1", "Agent", `{"prompt":"help","description":"helper","run_in_background":false,"script":"`+sub+`"}`))
	out, code := runInDir(t, dir, nil, "--script", orch, "--session-id", "hookerr-1", "--project-dir", dir, "--config-dir", cfg, "-p", "go")
	require.Equal(t, 0, code, out)

	var events []any
	for _, p := range payloads(t, log) {
		events = append(events, p["hook_event_name"])
	}
	assert.Equal(t, []any{"SubagentStop", "PostToolUse"}, events, "the sub-agent ran to its end")
	got := agentPosts(payloads(t, log))
	require.Len(t, got, 1)
	resp := got[0]["tool_response"].(map[string]any)
	// the mock's hand-back against the recorded one: the same status, the same
	// report as content, the same fields bar the ids, timings and model
	for _, k := range []string{"status", "content", "harnessNoteCount", "harnessTailCount", "totalToolUseCount"} {
		assert.Equal(t, want[k], resp[k], k)
	}
	assert.Equal(t, keysOf(want), keysOf(resp))
	assert.Contains(t, out, "HELPED")
}
