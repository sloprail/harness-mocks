package e2e

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT017_34_SubagentStopListsTheSessionsBackgroundTasks: a SubagentStop payload
// lists the session's still-running background tasks, not only the sub-agent's own
// (the recorded bgagent run: a background sub-agent's SubagentStop lists itself): a
// background command the main thread started shows as a "shell" entry, and the
// sub-agent's own transcript, last message and session_crons are in the payload.
// sr:proves subagent-lifecycle-hooks/claude
func TestT017_34_SubagentStopListsTheSessionsBackgroundTasks(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	settings(t, dir, map[string]string{"SubagentStop": payloadLogger(t, dir, "log.sh", log, "")})
	leaf := script(t, dir, "leaf", `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"leaf said this"}]}}`)
	sc := script(t, dir, "s",
		toolUse("bg", "Bash", `{"command":"sleep 30","description":"nap","run_in_background":true}`),
		toolUse("ag", "Agent", `{"prompt":"p","description":"helper","script":"`+leaf+`"}`),
	)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "st-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	ps := payloads(t, log)
	require.Len(t, ps, 1)
	p := ps[0]
	assert.Equal(t, "SubagentStop", p["hook_event_name"])
	tasks, _ := p["background_tasks"].([]any)
	require.Len(t, tasks, 1, "the main thread's background command is in the session's task list")
	entry := tasks[0].(map[string]any)
	assert.Equal(t, "shell", entry["type"])
	assert.Equal(t, "running", entry["status"])
	assert.Equal(t, "nap", entry["description"])
	assert.Equal(t, "sleep 30", entry["command"])
	assert.Equal(t, []any{}, p["session_crons"])
	assert.Contains(t, p["agent_transcript_path"], filepath.Join("subagents", "agent-"))
	assert.Equal(t, "leaf said this", p["last_assistant_message"])
}
