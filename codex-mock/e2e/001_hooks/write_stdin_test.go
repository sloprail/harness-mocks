package e2e

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A command left running by a call that yielded is polled with write_stdin: the poll waits for it, tells
// what it printed and its exit code, completes the command's item in the event stream and fires the
// PostToolUse of the call that started it, but no hook of its own for the poll; a session nobody was
// told of fails the call (recorded: runs/task-stream-frames).
func TestAYieldedCommandIsPolledToItsEnd(t *testing.T) {
	got := execMock(t, scenario{
		HooksJSON: hooksJSON("sh hook.sh", "PostToolUse"),
		Files:     map[string]string{"hook.sh": `cat >>"$HOOK_LOG"; echo >>"$HOOK_LOG"`},
		Script: `#!/bin/sh
n=$(grep -c function_call_output "$A10N_MOCK_SESSION_FILE")
case "$n" in
0) printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"c0","name":"Bash","input":{"command":"sleep 1; echo POLLED","yield_time_ms":50}}]}}' ;;
1) sid=$(jq -r 'select(.payload.type=="function_call_output")|.payload.output|fromjson|.session_id' "$A10N_MOCK_SESSION_FILE" | head -1)
   printf '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"c1","name":"write_stdin","input":{"session_id":%s,"yield_time_ms":10000}}]}}\n' "$sid" ;;
2) printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"c2","name":"write_stdin","input":{"session_id":1,"yield_time_ms":10}}]}}' ;;
*) printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"done"}]}}' '{"type":"result","subtype":"success","result":"done"}' ;;
esac
`, Prompt: "go"})
	require.Equal(t, 0, got.Code, got.Stderr)
	var started, completed int
	for _, e := range got.stream() {
		if item, _ := e["item"].(map[string]any); item["type"] == "command_execution" {
			if e["type"] == "item.started" {
				started++
			} else {
				completed++
				assert.Equal(t, "POLLED\n", item["aggregated_output"])
				assert.EqualValues(t, 0, item["exit_code"])
			}
		}
	}
	assert.Equal(t, []int{1, 1}, []int{started, completed}, "the command's item opened once and completed when the poll saw it end")
	post := eventsOf(got, "PostToolUse")
	require.Len(t, post, 1, "the started call's PostToolUse, once it ended; none for the polls")
	assert.Equal(t, "POLLED\n", post[0]["tool_response"])
	told := toolOutputs(t, got.rollout(t))
	require.Len(t, told, 3)
	assert.Contains(t, told[1], `"exit_code":0`)
	assert.Contains(t, told[1], `"output":"POLLED\n"`)
	assert.Contains(t, told[2], "Unknown process id 1")
}
