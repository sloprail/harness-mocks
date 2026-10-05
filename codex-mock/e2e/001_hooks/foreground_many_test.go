package e2e

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The agent spawns two sub-agents and makes one wait call naming both: the wait returns at the
// first to finish, tells only of that one (a target still running is not listed, nor among the
// receivers of the stream's completed wait item), and the session is told of each sub-agent's
// end one at a time, the second after the agent's answer, which the turn then goes on to
// (recorded: runs/foreground-subagent-wait-many; the doc says Codex waits for all of them).
const (
	twoSpawnsOneWait = `#!/bin/sh
n=$(grep -c function_call_output "$A10N_MOCK_SESSION_FILE")
k=$(grep -c '<subagent_notification' "$A10N_MOCK_SESSION_FILE")
ids=$(jq -rs '[.[]|select(.payload.type=="function_call_output")|.payload.output|try (fromjson|.agent_id) catch empty|select(.!=null)]|join(",")' "$A10N_MOCK_SESSION_FILE")
a=${ids%%,*}; b=${ids#*,}
case "$n" in
0) printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"s0","name":"spawn_agent","input":{"message":"fast","script":"fast.sh","more":true}}]}}';;
1) printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"s1","name":"spawn_agent","input":{"message":"slow","script":"slow.sh","more":true}}]}}';;
2) printf '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"w","name":"wait_agent","input":{"targets":["%s","%s"],"timeout_ms":60000}}]}}\n' "$a" "$b";;
*) if [ "$k" -le 1 ]; then t=FIRST; else t=SECOND; fi
   printf '%s\n' '{"gate":{"ended":[0,1]},"type":"assistant","message":{"content":[{"type":"text","text":"'$t'"}]}}' '{"type":"result","subtype":"success","result":"'$t'"}';;
esac
`
	fastSub = "#!/bin/sh\nprintf '%s\\n' '{\"type\":\"assistant\",\"message\":{\"content\":[{\"type\":\"text\",\"text\":\"PINEAPPLE-7\"}]}}'\n"
	// the second sub-agent ends only after the agent's wait call has finished: the script says so, no delay does
	slowSub = "#!/bin/sh\nprintf '%s\\n' '{\"gate\":{\"parent_done\":3},\"type\":\"assistant\",\"message\":{\"content\":[{\"type\":\"text\",\"text\":\"MANGO-3\"}]}}'\n"
)

// sr:proves foreground-subagent-result/codex
func TestAWaitForSeveralReturnsAtTheFirstAndTheOthersAreToldOfLater(t *testing.T) {
	rec := loadRecording(t, "foreground-subagent-wait-many")
	var wantWait map[string]any
	for _, l := range jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))) {
		if l["tool_name"] == "multi_agent_v1wait_agent" {
			wantWait = l
		}
	}
	require.NotNil(t, wantWait)
	var want struct {
		Status map[string]map[string]string `json:"status"`
	}
	require.NoError(t, json.Unmarshal([]byte(wantWait["tool_response"].(string)), &want))
	require.Len(t, want.Status, 1, "recording: only the sub-agent that finished is told of")

	got := execMock(t, scenario{Script: twoSpawnsOneWait, Files: map[string]string{"fast.sh": fastSub, "slow.sh": slowSub, "hook.sh": readFile(t, filepath.Join(rec.setup, "hook.sh"))},
		HooksJSON: readFile(t, filepath.Join(rec.setup, "hooks.json")),
		Prompt:    "Spawn two sub-agents and wait for them."})
	require.Equal(t, 0, got.Code, got.Stderr)
	var have struct {
		Status map[string]map[string]string `json:"status"`
	}
	for _, l := range got.hookLog() {
		if l["tool_name"] == "multi_agent_v1wait_agent" {
			require.NoError(t, json.Unmarshal([]byte(l["tool_response"].(string)), &have))
		}
	}
	require.Len(t, have.Status, 1, "mock: only the first to finish")
	for _, v := range have.Status {
		assert.Equal(t, "PINEAPPLE-7", v["completed"])
	}

	// the stream's completed wait item names only that sub-agent among its receivers
	for _, e := range got.stream() {
		if item, _ := e["item"].(map[string]any); item["tool"] == "wait" && e["type"] == "item.completed" {
			assert.Len(t, item["receiver_thread_ids"], 1)
		}
	}
	// the session is told of each sub-agent's end, and the turn answered twice: after each
	var answers []string
	for _, e := range got.stream() {
		if item, _ := e["item"].(map[string]any); item["type"] == "agent_message" {
			answers = append(answers, item["text"].(string))
		}
	}
	assert.Equal(t, []string{"FIRST", "SECOND"}, answers)
}
