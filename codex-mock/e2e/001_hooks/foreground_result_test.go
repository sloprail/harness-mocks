package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The recorded run runs/foreground-subagent-result: the agent spawns one
// sub-agent asking for the word PINEAPPLE-7, waits for it, and answers with
// what it reported.

const spawnOnly = `#!/bin/sh
n=$(grep -c function_call_output "$A10N_MOCK_SESSION_FILE")
if [ "$n" = 0 ]; then
  printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"call_0","name":"spawn_agent","input":{"message":"Reply with exactly the word PINEAPPLE-7 and nothing else.","script":"sub.sh"}}]}}'
  exit 0
fi
if [ "$n" = 1 ]; then
  printf '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"call_wait","name":"wait_agent","input":{"targets":["%s"],"timeout_ms":60000}}]}}\n' "$(jq -r 'select(.payload.type=="function_call_output")|.payload.output|try (fromjson|.agent_id) catch empty|select(.!=null)' "$A10N_MOCK_SESSION_FILE" | head -1)"
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"PINEAPPLE-7"}]}}' '{"type":"result","subtype":"success","result":"PINEAPPLE-7"}'
`

const pineappleSub = `#!/bin/sh
sleep 0.4
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"PINEAPPLE-7"}]}}'
`

func collabItems(events []map[string]any) (out []map[string]any) {
	for _, e := range events {
		if item, _ := e["item"].(map[string]any); item["type"] == "collab_tool_call" {
			out = append(out, map[string]any{"phase": e["type"], "item": item})
		}
	}
	return
}

// A sub-agent is spawned and waited for, and the parent is told its final
// report: the stream carries the recorded four collab items (spawn started and
// completed with the sub-agent pending, wait started and completed with it
// completed and the report as its message), only after the sub-agent has
// finished, and what the agent is told is the status map the recording shows
// for the wait, with the report as its completion. The recording has no usage
// trailer, and the mock adds none.
// sr:proves foreground-subagent-result/codex
func TestASubAgentIsWaitedForAndItsReportIsTheResultTheParentIsTold(t *testing.T) {
	rec := loadRecording(t, "foreground-subagent-result")
	want := collabItems(jsonLines(readFile(t, filepath.Join(rec.sample, "stream.jsonl"))))
	require.Len(t, want, 4)

	got := execMock(t, scenario{Script: spawnOnly, Files: map[string]string{"sub.sh": pineappleSub},
		Prompt: "Spawn one sub-agent and wait for it."})
	require.Equal(t, 0, got.Code, got.Stderr)
	calls := collabItems(got.stream())
	require.Len(t, calls, 4)

	for i := range want {
		wi, gi := want[i]["item"].(map[string]any), calls[i]["item"].(map[string]any)
		assert.Equal(t, want[i]["phase"], calls[i]["phase"])
		assert.Equal(t, wi["tool"], gi["tool"])
		assert.Equal(t, wi["status"], gi["status"])
		assert.Equal(t, keysOf(wi), keysOf(gi))
		assert.Equal(t, len(wi["receiver_thread_ids"].([]any)), len(gi["receiver_thread_ids"].([]any)), "receivers of item %d", i)
		assert.Equal(t, wi["prompt"] == nil, gi["prompt"] == nil)
	}
	child := calls[1]["item"].(map[string]any)["receiver_thread_ids"].([]any)[0].(string)
	assert.Equal(t, "pending_init", calls[1]["item"].(map[string]any)["agents_states"].(map[string]any)[child].(map[string]any)["status"])
	waited := calls[3]["item"].(map[string]any)["agents_states"].(map[string]any)[child].(map[string]any)
	assert.Equal(t, map[string]any{"status": "completed", "message": "PINEAPPLE-7"}, waited)

	var parent string
	_ = filepath.Walk(filepath.Join(got.Home, "sessions"), func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			if b, _ := os.ReadFile(p); strings.Contains(string(b), "function_call_output") {
				parent = string(b)
			}
		}
		return nil
	})
	assert.Contains(t, parent, `{\"status\":{\"`+child+`\":{\"completed\":\"PINEAPPLE-7\"}},\"timed_out\":false}`)
}
