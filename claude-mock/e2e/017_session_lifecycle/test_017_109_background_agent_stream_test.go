package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// kindsOf are the type/subtype of each stream frame, with the ones the mock never sends
// (thinking_tokens, commands_changed, rate_limit_event) left out and a run of assistant frames
// as one.
func kindsOf(frames []map[string]any) (out []string) {
	for _, f := range frames {
		k, _ := f["type"].(string)
		if sub, _ := f["subtype"].(string); sub != "" {
			k += "/" + sub
		}
		if k == "system/thinking_tokens" || k == "system/commands_changed" || k == "rate_limit_event" {
			continue
		}
		if k == "assistant" && len(out) > 0 && out[len(out)-1] == "assistant" {
			continue
		}
		out = append(out, k)
	}
	return out
}

// afterNotification is the kinds from the last task_notification frame on.
func afterNotification(kinds []string) []string {
	for i := len(kinds) - 1; i >= 0; i-- {
		if kinds[i] == "system/task_notification" {
			return kinds[i:]
		}
	}
	return nil
}

// TestT017_109_ResultsOfABackgroundAgentRunComeLast: a turn that ends while a background agent
// still works streams no result then; the agent's notification starts a turn that opens with an
// init frame, and the results of both turns follow that turn, in order, with result_index 0 and
// 1 (recording bgagent: task_notification, init, assistant, result, result).
// sr:proves print-waits-for-background-agents/claude
// sr:proves task-stream-frames/claude
func TestT017_109_ResultsOfABackgroundAgentRunComeLast(t *testing.T) {
	data, err := os.ReadFile(recordedFile(t, "../../snapshots/runs/bgagent/samples/*/stream.jsonl"))
	require.NoError(t, err)
	want := afterNotification(kindsOf(streamFrames(t, string(data))))
	require.Equal(t, []string{"system/task_notification", "system/init", "assistant", "result/success", "result/success"}, want, "recorded")

	dir := t.TempDir()
	// the agent finishes only after the first Stop has fired, so that turn ends with it still working
	h := payloadLogger(t, dir, "log.sh", filepath.Join(dir, "payloads.log"), `case "$IN" in *'"hook_event_name":"Stop"'*) touch `+dir+`/stopped;; esac`)
	settings(t, dir, map[string]string{"Stop": h})
	sub := write(t, filepath.Join(dir, "sub.sh"), `#!/bin/sh
while [ ! -f `+dir+`/stopped ]; do sleep 0.05; done
echo '{"type":"assistant","message":{"role":"assistant","stop_reason":"end_turn","content":[{"type":"text","text":"AGENT-REPLY"}]}}'
echo '{"type":"result","subtype":"success","result":"AGENT-REPLY"}'
`, 0o755)
	sc := write(t, filepath.Join(dir, "main.sh"), `#!/bin/sh
if ! grep -q 'turn-a' "$A10N_MOCK_SESSION_FILE" 2>/dev/null; then
cat <<'JSONL'
{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"ag1turn-a","name":"Agent","input":{"prompt":"go","description":"bg agent","script":"`+sub+`","run_in_background":true}}]}}
JSONL
exit 0
fi
echo '{"type":"assistant","message":{"role":"assistant","stop_reason":"end_turn","content":[{"type":"text","text":"LAUNCHED"}]}}'
echo '{"type":"result","subtype":"success","result":"LAUNCHED"}'
`, 0o755)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "bgs-1", "--project-dir", dir,
		"--config-dir", filepath.Join(dir, "config"), "--output-format", "stream-json", "-p", "hello")
	require.Equal(t, 0, code, out)
	frames := streamFrames(t, out)
	assert.Equal(t, want, afterNotification(kindsOf(frames)))
	var indexes []any
	for _, f := range frames {
		if f["type"] == "result" {
			indexes = append(indexes, f["result_index"])
		}
	}
	assert.Equal(t, []any{float64(0), float64(1)}, indexes)
}
