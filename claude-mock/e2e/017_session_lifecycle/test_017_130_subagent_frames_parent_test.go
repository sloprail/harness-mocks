package e2e

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// In a print run's stream a foreground sub-agent's assistant and user frames carry the
// tool_use id of the Agent call that started it as parent_tool_use_id, and the first is
// a user frame holding its prompt; the main thread's own frames carry none (recorded:
// snapshots/runs/include-hook-events-more, a sub-agent told "Reply only HELPED").
// sr:proves noninteractive-run/claude
func TestT017_130_SubagentFramesNameTheirParentToolUse(t *testing.T) {
	dir := t.TempDir()
	sub := callThenReply(t, dir, "sub", "HELPED",
		toolUse("b1", "Bash", `{"command":"true","description":"Do nothing"}`))
	orch := script(t, dir, "orch", toolUse("ag1", "Agent", `{"prompt":"Reply only HELPED","description":"d","run_in_background":false,"script":"`+sub+`"}`))
	out, code := runInDir(t, dir, nil, "--script", orch, "--session-id", "pf-1", "--project-dir", dir,
		"--config-dir", filepath.Join(dir, "config"), "--output-format", "stream-json", "-p", "hello")
	require.Equal(t, 0, code, out)

	var subFrames, mainFrames []map[string]any
	for _, l := range strings.Split(out, "\n") {
		var f map[string]any
		if json.Unmarshal([]byte(l), &f) != nil || (f["type"] != "assistant" && f["type"] != "user") {
			continue
		}
		if f["parent_tool_use_id"] != nil {
			subFrames = append(subFrames, f)
		} else {
			mainFrames = append(mainFrames, f)
		}
	}
	require.GreaterOrEqual(t, len(subFrames), 3, out)
	var agentCall string
	for _, f := range mainFrames {
		if strings.Contains(mustJSON(t, f["message"]), `"name":"Agent"`) {
			agentCall = f["message"].(map[string]any)["content"].([]any)[0].(map[string]any)["id"].(string)
		}
	}
	require.NotEmpty(t, agentCall, "the Agent tool_use frame")
	for _, f := range subFrames {
		assert.Equal(t, agentCall, f["parent_tool_use_id"])
	}
	first := subFrames[0]
	assert.Equal(t, "user", first["type"], "the sub-agent's first frame is its prompt")
	assert.Contains(t, mustJSON(t, first["message"]), "Reply only HELPED")
	assert.NotEmpty(t, mainFrames, "the main thread's frames carry no parent_tool_use_id")
}
