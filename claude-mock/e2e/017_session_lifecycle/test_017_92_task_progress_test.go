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

// subagentFrames are what a stream carries of one sub-agent, as "type/subtype"
// and, for the frames that name one, the tool: its prompt (a user frame), then
// before each of its tool calls a task_progress frame, the call and its result.
func subagentFrames(t *testing.T, stream string) (frames []string, progress []map[string]any) {
	t.Helper()
	for _, l := range strings.Split(stream, "\n") {
		var f map[string]any
		if json.Unmarshal([]byte(l), &f) != nil {
			continue
		}
		switch {
		case f["subtype"] == "task_progress":
			frames = append(frames, "progress")
			progress = append(progress, f)
		case f["parent_tool_use_id"] != nil && f["type"] == "user":
			frames = append(frames, "user")
		case f["parent_tool_use_id"] != nil && f["type"] == "assistant":
			frames = append(frames, "assistant")
		}
	}
	return
}

// A foreground sub-agent streams its prompt, then for each tool call a
// task_progress frame ("Running <the call's description>", the call count and the
// tool), the call and its result, all naming the call that started it
// (runs/isolated-worktree, three Bash calls); its final answer does not stream.
// sr:proves task-stream-frames/claude
func TestT017_92_ASubagentStreamsItsProgress(t *testing.T) {
	raw, err := os.ReadFile(recordedFile(t, "../../snapshots/runs/isolated-worktree/samples/*/stream.jsonl"))
	require.NoError(t, err)
	wantFrames, wantProgress := subagentFrames(t, string(raw))
	require.NotEmpty(t, wantProgress)

	dir := t.TempDir()
	sub := callThenReply(t, dir, "sub", "REPORT",
		toolUse("b1", "Bash", `{"command":"pwd","description":"Print working directory"}`),
		toolUse("b2", "Bash", `{"command":"true","description":"Do nothing"}`),
		toolUse("b3", "Bash", `{"command":"echo hi","description":"Say hi"}`),
	)
	orch := script(t, dir, "orch", toolUse("ag1", "Agent", `{"prompt":"go","description":"d","run_in_background":false,"script":"`+sub+`"}`))
	out, code := runInDir(t, dir, nil, "--script", orch, "--session-id", "prog-1", "--project-dir", dir,
		"--config-dir", filepath.Join(dir, "config"), "--output-format", "stream-json", "-p", "hello")
	require.Equal(t, 0, code, out)
	gotFrames, gotProgress := subagentFrames(t, out)
	assert.Equal(t, wantFrames, gotFrames, "the sub-agent's frames, in the recorded order")
	require.Len(t, gotProgress, 3)
	for i, p := range gotProgress {
		assert.Equal(t, keysOf(wantProgress[0]), keysOf(p))
		assert.Equal(t, float64(i+1), p["usage"].(map[string]any)["tool_uses"])
		assert.Equal(t, "Bash", p["last_tool_name"])
	}
	assert.Equal(t, "Running Print working directory", gotProgress[0]["description"])
	assert.Equal(t, "Running Do nothing", gotProgress[1]["description"])
	assert.NotContains(t, out, "REPORT\"}],\"role\":\"assistant\"", "the sub-agent's final answer is not a frame of its own")
}
