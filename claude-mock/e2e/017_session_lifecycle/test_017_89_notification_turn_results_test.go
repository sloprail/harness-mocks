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

// resultsOf are the result frames of a stream, in order.
func resultsOf(stream string) (results []map[string]any) {
	for _, l := range strings.Split(stream, "\n") {
		var f map[string]any
		if json.Unmarshal([]byte(l), &f) == nil && f["type"] == "result" {
			results = append(results, f)
		}
	}
	return
}

// A background agent's notification starts a turn of its own, and every turn
// ends with a result frame: the stream carries one result for the turn that
// launched the agent and one for the notification's turn, in that order, the
// second after the task's own frames (runs/bgagent: two results, "LAUNCHED" and
// the agent's completion).
// sr:proves background-agent/claude
// sr:proves task-stream-frames/claude
func TestT017_89_ANotificationTurnEndsWithItsOwnResult(t *testing.T) {
	recorded, err := os.ReadFile(recordedFile(t, "../../snapshots/runs/bgagent/samples/*/stream.jsonl"))
	require.NoError(t, err)
	want := resultsOf(string(recorded))
	require.Len(t, want, 2, "recorded: one result per turn")

	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	sub := replyScript(t, dir, "sub", "AGENT-REPLY")
	sc := write(t, filepath.Join(dir, "s.sh"), `#!/bin/sh
F="$A10N_MOCK_SESSION_FILE"
if ! grep -q '"tool_result"' "$F"; then
cat <<'JSONL'
`+strings.Replace(toolUse("ag1", "Agent", `{"prompt":"go","description":"bg agent","script":"`+sub+`","run_in_background":true}`), "@MARK@", "", 1)+`
JSONL
exit 0
fi
if ! grep -q 'task-notification' "$F"; then
  echo '{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"LAUNCHED"}]}}'
  echo '{"type":"result","subtype":"success","result":"LAUNCHED"}'
  exit 0
fi
echo '{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"COMPLETED"}]}}'
echo '{"type":"result","subtype":"success","result":"COMPLETED"}'
`, 0o755)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "notif-1", "--project-dir", dir, "--config-dir", cfg,
		"--output-format", "stream-json", "-p", "hello")
	require.Equal(t, 0, code, out)
	got := resultsOf(out)
	require.Len(t, got, 2, out)
	assert.Equal(t, "LAUNCHED", got[0]["result"])
	assert.Equal(t, "COMPLETED", got[1]["result"])
	idx := func(sub string) int { return strings.Index(out, sub) }
	assert.Less(t, idx(`"subtype":"task_notification"`), strings.LastIndex(out, `"type":"result"`), "the second result follows the task's own frames")
	assert.Equal(t, want[0]["subtype"], got[0]["subtype"])
	assert.Equal(t, want[1]["subtype"], got[1]["subtype"])
}
