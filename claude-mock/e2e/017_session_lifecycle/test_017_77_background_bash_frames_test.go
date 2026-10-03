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

// taskFrameSubtypes are the stream frames that follow a background task.
var taskFrameSubtypes = map[string]bool{
	"background_tasks_changed": true, "task_started": true, "task_updated": true, "task_notification": true,
}

// streamTaskFrames are the task frames among stream lines, in order, and
// whether the result frame came after all of them.
func streamTaskFrames(t *testing.T, stream string) (frames []map[string]any, beforeResult bool) {
	t.Helper()
	resultSeen := false
	beforeResult = true
	for _, l := range strings.Split(stream, "\n") {
		var m map[string]any
		if json.Unmarshal([]byte(l), &m) != nil {
			continue
		}
		if m["type"] == "result" {
			resultSeen = true
		}
		if s, _ := m["subtype"].(string); m["type"] == "system" && taskFrameSubtypes[s] {
			frames = append(frames, m)
			beforeResult = beforeResult && !resultSeen
		}
	}
	return frames, beforeResult
}

// TestT017_77_BackgroundBashFinishingOnItsOwnStreamsItsFrames: a background
// command that finishes by itself while the agent works streams, in the order
// of the recorded midturn run, background_tasks_changed (the task listed with
// its id, task_type local_bash and description), task_started
// (is_backgrounded true, tool_use_id of the call), background_tasks_changed
// (empty), task_updated (patch status completed with an end_time) and a
// completed task_notification (output_file, summary 'Background command "<d>"
// completed (exit code 0)'), all before the result, each with the fields of
// the recorded frame.
// sr:proves background-bash/claude
func TestT017_77_BackgroundBashFinishingOnItsOwnStreamsItsFrames(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	sc := script(t, dir, "s",
		toolUse("bg1", "Bash", `{"command":"echo QUICKDONE","run_in_background":true}`),
		toolUse("fg1", "Bash", `{"command":"sleep 2"}`),
	)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "bgf-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)

	raw, err := os.ReadFile(filepath.Join(recordedSample(t, "midturn"), "stream.jsonl"))
	require.NoError(t, err)
	want, wantBefore := streamTaskFrames(t, string(raw))
	got, gotBefore := streamTaskFrames(t, out)
	require.True(t, wantBefore)
	assert.True(t, gotBefore, "every task frame comes before the result")
	require.Len(t, want, 5)
	subtypes := func(fs []map[string]any) (s []any) {
		for _, f := range fs {
			s = append(s, f["subtype"])
		}
		return s
	}
	require.Equal(t, subtypes(want), subtypes(got), "the frames come in the recorded order")
	for i := range want {
		assert.Equal(t, keysOf(want[i]), keysOf(got[i]), "fields of frame %d (%v)", i, want[i]["subtype"])
	}

	id, _ := got[1]["task_id"].(string)
	require.Regexp(t, `^b[a-z0-9]{8}$`, id)
	assert.Equal(t, []any{map[string]any{"task_id": id, "task_type": "local_bash", "description": "echo QUICKDONE"}}, got[0]["tasks"])
	assert.Equal(t, true, got[1]["is_backgrounded"])
	assert.Equal(t, "local_bash", got[1]["task_type"])
	assert.Equal(t, "echo QUICKDONE", got[1]["description"])
	assert.Equal(t, "bg1turn-s-a", got[1]["tool_use_id"])
	assert.Equal(t, []any{}, got[2]["tasks"])
	assert.Equal(t, id, got[3]["task_id"])
	patch := got[3]["patch"].(map[string]any)
	assert.Equal(t, "completed", patch["status"])
	assert.IsType(t, float64(0), patch["end_time"])
	assert.Equal(t, keysOf(want[3]["patch"].(map[string]any)), keysOf(patch))
	note := got[4]
	assert.Equal(t, "completed", note["status"])
	assert.Equal(t, id, note["task_id"])
	assert.Equal(t, "bg1turn-s-a", note["tool_use_id"])
	assert.Equal(t, `Background command "echo QUICKDONE" completed (exit code 0)`, note["summary"])
	assert.True(t, strings.HasSuffix(note["output_file"].(string), "/tasks/"+id+".output"), note["output_file"])
	assert.Equal(t, want[4]["summary"], note["summary"])
}
