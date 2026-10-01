package e2e

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT017_37_BackgroundCommandFramesThroughTheBinary: the stream of a run that starts a
// background command carries, in the recorded order (snapshots/runs/bgbash and
// midturn): background_tasks_changed with the command listed ({task_id, task_type
// local_bash, description}), task_started {tool_use_id, description,
// is_backgrounded true, task_type local_bash}, then when it ends
// background_tasks_changed with none, task_updated {patch.status, end_time} and
// task_notification {tool_use_id, status, output_file, summary}.
// sr:proves task-stream-frames/claude
func TestT017_37_BackgroundCommandFramesThroughTheBinary(t *testing.T) {
	dir := t.TempDir()
	tmp := filepath.Join(dir, "tmp")
	sc := script(t, dir, "s",
		toolUse("bg", "Bash", `{"command":"echo QUICK","description":"quick one","run_in_background":true}`),
		toolUse("fg", "Bash", `{"command":"sleep 1"}`),
	)
	out, code := runInDir(t, dir, []string{"CLAUDE_CODE_TMPDIR=" + tmp}, "--script", sc, "--session-id", "tf-1",
		"--project-dir", dir, "--config-dir", filepath.Join(dir, "config"), "-p", "hello")
	require.Equal(t, 0, code, out)
	var kinds []string
	frames := map[string]map[string]any{}
	var changed []any
	for _, l := range strings.Split(out, "\n") {
		var f map[string]any
		if json.Unmarshal([]byte(l), &f) != nil || f["type"] != "system" {
			continue
		}
		sub, _ := f["subtype"].(string)
		kinds = append(kinds, sub)
		if sub == "background_tasks_changed" {
			changed = append(changed, f["tasks"])
		} else {
			frames[sub] = f
		}
	}
	assert.Equal(t, []string{"background_tasks_changed", "task_started", "background_tasks_changed", "task_updated", "task_notification"}, kinds)
	id := frames["task_started"]["task_id"].(string)
	assert.Equal(t, []any{map[string]any{"task_id": id, "task_type": "local_bash", "description": "quick one"}}, changed[0])
	assert.Equal(t, []any{}, changed[1])
	started := frames["task_started"]
	assert.Equal(t, "quick one", started["description"])
	assert.Equal(t, true, started["is_backgrounded"])
	assert.Equal(t, "local_bash", started["task_type"])
	assert.Equal(t, "bgturn-s-a", started["tool_use_id"])
	assert.Equal(t, "completed", frames["task_updated"]["patch"].(map[string]any)["status"])
	assert.NotZero(t, frames["task_updated"]["patch"].(map[string]any)["end_time"])
	note := frames["task_notification"]
	assert.Equal(t, "completed", note["status"])
	assert.Equal(t, "bgturn-s-a", note["tool_use_id"])
	assert.Equal(t, `Background command "quick one" completed (exit code 0)`, note["summary"])
	assert.Contains(t, note["output_file"], filepath.Join("tf-1", "tasks", id+".output"))
}
