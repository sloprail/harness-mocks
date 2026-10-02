package e2e

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT017_66_ForegroundAgentBlocksItsParent: a foreground Agent call does not
// return until the sub-agent has finished: the parent's next step runs after
// the sub-agent's work is done, and gets the sub-agent's report as the result.
// sr:proves foreground-subagent-result/claude
func TestT017_66_ForegroundAgentBlocksItsParent(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	done, saw := filepath.Join(dir, "SUBDONE"), filepath.Join(dir, "PARENTSAW")
	sub := write(t, filepath.Join(dir, "sub.sh"), `#!/bin/sh
sleep 1
touch `+done+`
echo '{"type":"assistant","message":{"role":"assistant","stop_reason":"end_turn","content":[{"type":"text","text":"SLOW-REPORT-6601"}]}}'
echo '{"type":"result","subtype":"success","result":"SLOW-REPORT-6601"}'
`, 0o755)
	sc := script(t, dir, "s",
		toolUse("ag1", "Agent", `{"prompt":"go","description":"slow agent","script":"`+sub+`"}`),
		toolUse("b2", "Bash", `{"command":"test -f `+done+` && touch `+saw+`"}`),
	)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "fgb-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	assert.FileExists(t, saw, "the parent's next step ran only after the sub-agent had finished")
	block, _ := toolResultOf(t, readRecs(t, transcriptPath(t, cfg, dir, "fgb-1")), "ag1turn-s-a")
	assert.Contains(t, block["content"].([]any)[0].(map[string]any)["text"], "SLOW-REPORT-6601")
}

// TestT017_65_BackgroundAgentHooks: the hooks around a background Agent, in
// the order recorded (snapshots/runs/bgagent, payloads.jsonl): PreToolUse of
// the Agent call, SubagentStart, the Agent's PostToolUse carrying the
// async_launched payload, the sub-agent's own Bash PreToolUse (agent_id and
// agent_type set) before the launching turn's Stop, its PostToolUse after it,
// SubagentStop (agent_transcript_path is the sub-agent's own transcript, its
// last_assistant_message the reply, and background_tasks still lists the
// sub-agent as running), the notification's UserPromptSubmit, and the second
// Stop.
// sr:proves background-agent/claude
func TestT017_65_BackgroundAgentHooks(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	h := payloadLogger(t, dir, "log.sh", log, "")
	// The launching turn's Stop is slow to log, so the sub-agent's own work is
	// seen to happen while that turn is still live.
	slow := write(t, filepath.Join(dir, "slow.sh"), "#!/bin/sh\nIN=$(cat)\nsleep 1\nprintf '%s\\n' \"$IN\" >> "+log+"\n", 0o755)
	settings(t, dir, map[string]string{
		"UserPromptSubmit": h, "PreToolUse": h, "PostToolUse": h, "SubagentStart": h, "SubagentStop": h, "Stop": slow,
	})
	sub := script(t, dir, "sub", toolUse("sb", "Bash", `{"command":"sleep 3; echo SUBDONE"}`))
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
	// The sub-agent works while the launching turn is live: its Bash PreToolUse
	// comes before the first Stop and its PostToolUse after it.
	assert.Equal(t, []string{
		"UserPromptSubmit", "PreToolUse:Agent", "SubagentStart", "PostToolUse:Agent", "PreToolUse:Bash", "Stop",
		"PostToolUse:Bash", "SubagentStop", "UserPromptSubmit", "Stop",
	}, order)

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
	assert.Equal(t, "done", stop["last_assistant_message"])
	for _, label := range []string{"PreToolUse:Bash", "PostToolUse:Bash"} {
		p := byEvent[label][0]
		assert.Equal(t, agentID, p["agent_id"], label+" names the sub-agent")
		assert.Equal(t, "general-purpose", p["agent_type"], label)
	}
	assert.Equal(t, []any{map[string]any{"id": agentID, "type": "subagent", "status": "running", "description": "hook agent", "agent_type": "general-purpose"}},
		stop["background_tasks"], "the sub-agent is still listed running when its own SubagentStop fires")
}
