package e2e

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT017_65_BackgroundAgentHooks: the hooks around a background Agent, in
// the order recorded (snapshots/runs/bgagent, payloads.jsonl): PreToolUse of
// the Agent call, SubagentStart (its place differs, see below), the Agent's PostToolUse carrying the
// async_launched payload, the first Stop (which lists the sub-agent running),
// SubagentStop (agent_transcript_path is the sub-agent's own transcript, its
// last_assistant_message the reply, and background_tasks still lists the
// sub-agent as running), the notification's UserPromptSubmit, and the second
// Stop.
// sr:proves background-agent/claude
func TestT017_65_BackgroundAgentHooks(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	h := payloadLogger(t, dir, "log.sh", log, `case "$IN" in *'"hook_event_name":"Stop"'*) touch `+dir+`/stopped;; esac`)
	settings(t, dir, map[string]string{
		"UserPromptSubmit": h, "PreToolUse": h, "PostToolUse": h, "SubagentStart": h, "SubagentStop": h, "Stop": h,
	})
	sub := write(t, filepath.Join(dir, "sub.sh"), `#!/bin/sh
while [ ! -f `+dir+`/stopped ]; do sleep 0.05; done
echo '{"type":"assistant","message":{"role":"assistant","stop_reason":"end_turn","content":[{"type":"text","text":"AGENT-REPLY-6501"}]}}'
echo '{"type":"result","subtype":"success","result":"AGENT-REPLY-6501"}'
`, 0o755)
	input := `{"prompt":"go","description":"hook agent","script":"` + sub + `","run_in_background":true}`
	sc := script(t, dir, "s",
		toolUse("ag1", "Agent", input),
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"LAUNCHED @MARK@"}]}}`,
	)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "bgh-2",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)

	var order []string
	byEvent := map[string][]map[string]any{}
	for _, p := range payloads(t, log) {
		ev, _ := p["hook_event_name"].(string)
		if p["agent_id"] == nil && p["tool_name"] != nil && p["tool_name"] != "Agent" {
			continue
		}
		label := ev
		if p["tool_name"] != nil {
			label += ":" + p["tool_name"].(string)
		}
		order = append(order, label)
		byEvent[label] = append(byEvent[label], p)
	}
	// SubagentStart is left out of the order: the recording has it before the
	// Agent's PostToolUse, the mock fires it when the sub-agent's run starts,
	// after the launching turn's PostToolUse and Stop.
	var rest []string
	for _, l := range order {
		if l != "SubagentStart" {
			rest = append(rest, l)
		}
	}
	assert.Equal(t, []string{
		"UserPromptSubmit", "PreToolUse:Agent", "PostToolUse:Agent", "Stop", "SubagentStop", "UserPromptSubmit", "Stop",
	}, rest)
	require.Len(t, byEvent["SubagentStart"], 1)

	start := byEvent["SubagentStart"][0]
	agentID, _ := start["agent_id"].(string)
	require.Regexp(t, `^a[0-9a-f]{16}$`, agentID)
	assert.Equal(t, "general-purpose", start["agent_type"])

	post := byEvent["PostToolUse:Agent"][0]
	resp, _ := post["tool_response"].(map[string]any)
	require.NotNil(t, resp)
	assert.Equal(t, true, resp["isAsync"])
	assert.Equal(t, "async_launched", resp["status"])
	assert.Equal(t, agentID, resp["agentId"])
	assert.Equal(t, "hook agent", resp["description"])
	assert.Equal(t, "go", resp["prompt"])
	assert.Equal(t, true, resp["canReadOutputFile"])
	assert.True(t, strings.HasSuffix(resp["outputFile"].(string), "/tasks/"+agentID+".output"), resp["outputFile"])
	assert.Equal(t, true, post["tool_input"].(map[string]any)["run_in_background"])

	main := transcriptPath(t, cfg, dir, "bgh-2")
	stop := byEvent["SubagentStop"][0]
	assert.Equal(t, agentID, stop["agent_id"])
	assert.Equal(t, "general-purpose", stop["agent_type"])
	assert.Equal(t, false, stop["stop_hook_active"])
	assert.Equal(t, filepath.Join(strings.TrimSuffix(main, ".jsonl"), "subagents", "agent-"+agentID+".jsonl"), stop["agent_transcript_path"])
	assert.Equal(t, "AGENT-REPLY-6501", stop["last_assistant_message"])
	assert.Equal(t, []any{map[string]any{"id": agentID, "type": "subagent", "status": "running", "description": "hook agent", "agent_type": "general-purpose"}},
		stop["background_tasks"], "the sub-agent is still listed running when its own SubagentStop fires")
}
