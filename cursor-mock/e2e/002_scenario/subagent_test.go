package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// frames parses stream-json lines, skipping what is not an object.
func frames(text string) (out []map[string]any) {
	for _, l := range strings.Split(strings.TrimSpace(text), "\n") {
		var f map[string]any
		if json.Unmarshal([]byte(l), &f) == nil {
			out = append(out, f)
		}
	}
	return
}

// taskFrames are the frames of the Task tool call, by subtype.
func taskFrames(fs []map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, f := range fs {
		if call, ok := f["tool_call"].(map[string]any); ok && call["taskToolCall"] != nil {
			out[f["subtype"].(string)] = call["taskToolCall"].(map[string]any)
		}
	}
	return out
}

func keys(m map[string]any) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// The recorded run runs/foreground-subagent-result: the agent launches one
// foreground sub-agent with the Task tool, asking for the word PINEAPPLE-7.
//
// TestAForegroundSubAgentBlocksItsParentAndItsReportIsTheCallsResult: the
// parent does nothing until the sub-agent has finished (its next message
// follows the call's completed frame, which says how long the sub-agent took),
// and the completed frame holds the sub-agent's final report as the call's
// result, in the fields the recording shows: the report as the last
// conversation step, the sub-agent's id, not in the background, its duration.
// sr:proves foreground-subagent-result/cursor
func TestAForegroundSubAgentBlocksItsParentAndItsReportIsTheCallsResult(t *testing.T) {
	var rec []map[string]any
	samples, err := filepath.Glob("../../snapshots/runs/foreground-subagent-result/samples/*/stream.jsonl")
	require.NoError(t, err)
	require.NotEmpty(t, samples)
	b, err := os.ReadFile(samples[len(samples)-1])
	require.NoError(t, err)
	rec = frames(string(b))
	want := taskFrames(rec)
	require.Equal(t, false, want["completed"]["result"].(map[string]any)["success"].(map[string]any)["isBackground"])

	sub := filepath.Join(t.TempDir(), "sub.sh")
	require.NoError(t, os.WriteFile(sub, []byte(`#!/bin/sh
sleep 0.4
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"PINEAPPLE-7"}]}}'
`), 0o755))
	task := `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tu_1","name":"Task","input":{"description":"Reply PINEAPPLE-7 only","prompt":"Reply with exactly the word PINEAPPLE-7","script":"` + sub + `"}}]}}`
	out, stderr, code := run(t, `#!/bin/sh
if grep -q tool_use "$A10N_MOCK_SESSION_FILE"; then
  printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"PINEAPPLE-7"}]}}' '`+done+`'
else
  printf '%s\n' '`+task+`'
fi
`, "launch one")
	require.Equal(t, 0, code, stderr)
	got := frames(out)
	gotTask := taskFrames(got)
	require.Contains(t, gotTask, "started")
	require.Contains(t, gotTask, "completed")

	// the same fields as the recording: the call's args, and its success result
	gotArgs, wantArgs := gotTask["started"]["args"].(map[string]any), want["started"]["args"].(map[string]any)
	for _, k := range keys(gotArgs) {
		assert.Contains(t, wantArgs, k, "an arg the recording shows")
	}
	assert.Equal(t, wantArgs["description"], gotArgs["description"])
	wantSuccess := want["completed"]["result"].(map[string]any)["success"].(map[string]any)
	gotSuccess := gotTask["completed"]["result"].(map[string]any)["success"].(map[string]any)
	assert.Equal(t, keys(wantSuccess), keys(gotSuccess))
	assert.Equal(t, false, gotSuccess["isBackground"])
	assert.Equal(t, wantSuccess["conversationSteps"], gotSuccess["conversationSteps"], "the report is the sub-agent's last words")
	assert.NotEmpty(t, gotSuccess["agentId"])
	assert.Equal(t, wantSuccess["backgroundReason"], gotSuccess["backgroundReason"])
	ms, err := strconv.Atoi(gotSuccess["durationMs"].(string))
	require.NoError(t, err)
	assert.GreaterOrEqual(t, ms, 400, "the call lasted as long as the sub-agent ran")

	// the parent blocked: the call ended before its next message, and the
	// run's result carries the report it then gave
	var order []string
	for _, f := range got {
		switch f["type"] {
		case "tool_call":
			order = append(order, "tool_call/"+f["subtype"].(string))
		case "assistant", "result":
			order = append(order, f["type"].(string))
		}
	}
	assert.Equal(t, []string{"tool_call/started", "tool_call/completed", "assistant", "result"}, order)
}
