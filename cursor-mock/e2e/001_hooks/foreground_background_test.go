package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// taskSuccesses are the success results of the Task calls' completed frames of a
// stream, in order.
func taskSuccesses(frames []map[string]any) (out []map[string]any) {
	for _, f := range frames {
		tc, _ := f["tool_call"].(map[string]any)
		body, _ := tc["taskToolCall"].(map[string]any)
		if body == nil || f["subtype"] != "completed" {
			continue
		}
		if r, _ := body["result"].(map[string]any); r != nil {
			if s, _ := r["success"].(map[string]any); s != nil {
				out = append(out, s)
			}
		}
	}
	return out
}

// TestAForegroundTaskBlocksAndABackgroundOneReturnsAtOnce: the docs
// (#foreground-vs-background) say a foreground sub-agent blocks until it is
// done and a background one returns immediately; recorded
// (runs/foreground-subagent-result, runs/background-agent), the foreground
// call's result holds the sub-agent's report (one conversation step), isBackground
// false and the reason UNSPECIFIED, and the duration the sub-agent took; the
// background call's result holds no steps, isBackground true, the reason
// SUBAGENT_BACKGROUND_REASON_AGENT_REQUEST and a duration of a moment. The mock's
// two calls give the same contrast.
// sr:proves foreground-subagent-result/cursor
func TestAForegroundTaskBlocksAndABackgroundOneReturnsAtOnce(t *testing.T) {
	fg := taskSuccesses(readJSONL(t, filepath.Join(newestSample(t, "foreground-subagent-result"), "stream.jsonl")))
	bg := taskSuccesses(readJSONL(t, filepath.Join(newestSample(t, "background-agent"), "stream.jsonl")))
	require.Len(t, fg, 1)
	require.Len(t, bg, 1)
	assert.Equal(t, false, fg[0]["isBackground"])
	assert.Equal(t, "SUBAGENT_BACKGROUND_REASON_UNSPECIFIED", fg[0]["backgroundReason"])
	assert.Len(t, fg[0]["conversationSteps"], 1)
	assert.Equal(t, true, bg[0]["isBackground"])
	assert.Equal(t, "SUBAGENT_BACKGROUND_REASON_AGENT_REQUEST", bg[0]["backgroundReason"])
	assert.Empty(t, bg[0]["conversationSteps"])

	scratch, ws := t.TempDir(), t.TempDir()
	sub := filepath.Join(scratch, "sub.sh")
	require.NoError(t, os.WriteFile(sub, []byte(`#!/bin/sh
sleep 1
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"SUB-REPLY"}]}}' '{"type":"result","subtype":"success","result":"SUB-REPLY"}'
`), 0o755))
	main := filepath.Join(scratch, "main.sh")
	require.NoError(t, os.WriteFile(main, []byte(`#!/bin/sh
n=$(grep -c '"type":"tool_use"' "$A10N_MOCK_SESSION_FILE" 2>/dev/null)
case "${n:-0}" in
0) printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Task","input":{"description":"fg","prompt":"p","subagent_type":"generalPurpose","script":"`+sub+`"}}]}}' ;;
1) printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t2","name":"Task","input":{"description":"bg","prompt":"p","subagent_type":"generalPurpose","run_in_background":true,"script":"`+sub+`"}}]}}' ;;
*) printf '%s\n' '{"type":"result","subtype":"success","result":"DONE"}' ;;
esac
`), 0o755))
	cmd := exec.Command(binary, "-p", "--force", "--trust", "--output-format", "stream-json", "--script", main, "go")
	cmd.Dir, cmd.Env = ws, []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir()}
	out, err := cmd.Output()
	require.NoError(t, err, string(out))
	got := taskSuccesses(readJSONLText(t, string(out)))
	require.Len(t, got, 2)
	assert.Equal(t, false, got[0]["isBackground"])
	assert.Equal(t, "SUBAGENT_BACKGROUND_REASON_UNSPECIFIED", got[0]["backgroundReason"])
	assert.Len(t, got[0]["conversationSteps"], 1)
	ms, err := strconv.Atoi(got[0]["durationMs"].(string))
	require.NoError(t, err)
	assert.GreaterOrEqual(t, ms, 1000, "the foreground call lasted as long as the sub-agent ran")
	assert.Equal(t, true, got[1]["isBackground"])
	assert.Equal(t, "SUBAGENT_BACKGROUND_REASON_AGENT_REQUEST", got[1]["backgroundReason"])
	assert.Empty(t, got[1]["conversationSteps"])
}
