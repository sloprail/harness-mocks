package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT017_75_ForkRefusalFiresTheFailureHookAsRecorded: the refused dispatch is
// a tool call that ran and failed, so, as in the nested-fork-limit run, the
// fork's own PreToolUse for it is followed by a PostToolUseFailure carrying the
// refusal as its error (is_interrupt false), both reporting agent_type "fork",
// and the fork's SubagentStop comes after.
// sr:proves nested-subagents/claude
func TestT017_75_ForkRefusalFiresTheFailureHookAsRecorded(t *testing.T) {
	type call struct{ hook, agentType, tool string }
	var recorded []call
	var recordedError string
	raw, err := os.ReadFile(recordedFile(t, "../../snapshots/runs/nested-fork-limit/samples/*/events.jsonl"))
	require.NoError(t, err)
	for _, l := range strings.Split(string(raw), "\n") {
		var e struct {
			Hook    string
			Payload map[string]any
		}
		if json.Unmarshal([]byte(l), &e) != nil || e.Hook == "" || e.Payload["agent_type"] != "fork" {
			continue
		}
		tool, _ := e.Payload["tool_name"].(string)
		recorded = append(recorded, call{e.Hook, "fork", tool})
		if e.Hook == "PostToolUseFailure" {
			recordedError, _ = e.Payload["error"].(string)
			assert.Equal(t, false, e.Payload["is_interrupt"])
		}
	}
	require.Equal(t, []call{{"SubagentStart", "fork", ""}, {"PreToolUse", "fork", "Agent"}, {"PostToolUseFailure", "fork", "Agent"}, {"SubagentStop", "fork", ""}}, recorded)

	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	h := payloadLogger(t, dir, "log.sh", log, "")
	settings(t, dir, map[string]string{"SubagentStart": h, "SubagentStop": h, "PreToolUse": h, "PostToolUseFailure": h})
	inner := script(t, dir, "inner")
	mid := script(t, dir, "mid", toolUse("m1", "Agent", `{"prompt":"deeper","description":"inner","subagent_type":"general-purpose","script":"`+inner+`"}`))
	root := script(t, dir, "root", toolUse("r1", "Agent", `{"prompt":"layer","description":"outerfork","subagent_type":"fork","script":"`+mid+`"}`))
	out, code := runInDir(t, dir, []string{"CLAUDE_CODE_MAX_SUBAGENT_SPAWN_DEPTH=1"}, "--script", root, "--session-id", "fl-hooks",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	var got []call
	for _, p := range payloads(t, log) {
		if p["agent_type"] != "fork" {
			continue
		}
		tool, _ := p["tool_name"].(string)
		got = append(got, call{p["hook_event_name"].(string), "fork", tool})
		if p["hook_event_name"] == "PostToolUseFailure" {
			assert.Equal(t, recordedError, p["error"], "the refusal is the failure's error")
			assert.Equal(t, false, p["is_interrupt"])
		}
	}
	assert.Equal(t, recorded, got)
}
