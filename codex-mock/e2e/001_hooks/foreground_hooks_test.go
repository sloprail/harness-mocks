package e2e

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// spawnWaitShape is a hook payload reduced to what the capability is about: its
// event, the tool it names, the keys of its tool input, and the shape of the
// tool's answer (the receipt's keys, or the wait's status map and timed_out).
func spawnWaitShape(p map[string]any) map[string]any {
	out := map[string]any{"event": p["hook_event_name"], "tool": p["tool_name"]}
	if in, ok := p["tool_input"].(map[string]any); ok {
		delete(in, "script") // the mock's own parameter: the scenario script of the sub-agent
		out["input"] = keysOf(in)
	}
	if text, ok := p["tool_response"].(string); ok {
		var resp map[string]any
		if json.Unmarshal([]byte(text), &resp) == nil {
			out["response"] = keysOf(resp)
			if status, ok := resp["status"].(map[string]any); ok {
				for _, v := range status {
					out["completed"] = v
				}
			}
		}
	}
	return out
}

// The hooks of a spawn and the wait that follows it, as recorded
// (runs/foreground-subagent-result): PostToolUse for spawn_agent with the
// receipt as its response, PostToolUse for the wait, named
// multi_agent_v1wait_agent, with targets and timeout_ms and the status map as
// its response, then Stop, in that order.
// sr:proves foreground-subagent-result/codex
func TestTheHooksOfASpawnAndItsWaitAreTheRecordedOnes(t *testing.T) {
	rec := loadRecording(t, "foreground-subagent-result")
	var want []map[string]any
	for _, l := range jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))) {
		want = append(want, spawnWaitShape(l))
	}
	require.Len(t, want, 3)

	got := execMock(t, scenario{Script: spawnOnly, Files: map[string]string{"sub.sh": pineappleSub, "hook.sh": readFile(t, filepath.Join(rec.setup, "hook.sh"))},
		HooksJSON: readFile(t, filepath.Join(rec.setup, "hooks.json")),
		Prompt:    "Spawn one sub-agent and wait for it."})
	require.Equal(t, 0, got.Code, got.Stderr)
	var have []map[string]any
	for _, l := range got.hookLog() {
		have = append(have, spawnWaitShape(l))
	}
	assert.Equal(t, want, have)
	assert.Equal(t, "multi_agent_v1wait_agent", have[1]["tool"])
}
