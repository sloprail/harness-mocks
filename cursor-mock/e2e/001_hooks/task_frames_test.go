package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The recorded run runs/task-stream-frames: one prompt that has a sub-agent
// answer PONG and starts `sleep 5; echo BGDONE` as a background command, then
// waits for it.

// A background command is announced by its call's frame (completed at once,
// isBackground, naming the task by its shellId) and ends with a system
// task_notification frame of the same id, a status and the command's
// description as title. Nothing else on the stream is a frame of a task: no
// task_started or task_updated (runs/task-stream-frames).
// sr:proves task-notifications/cursor
func TestABackgroundCommandIsAnnouncedByItsCallAndEndsWithANotificationFrame(t *testing.T) {
	// what the real run showed
	var wantNote, wantShell map[string]any
	for _, f := range recordedStream(t, "task-stream-frames") {
		switch {
		case f["type"] == "system":
			assert.Contains(t, []any{"init", "task_notification"}, f["subtype"], "the real run has no other task frame")
			if f["subtype"] == "task_notification" {
				wantNote = f
			}
		case f["type"] == "tool_call" && f["subtype"] == "completed":
			if c, ok := f["tool_call"].(map[string]any)["shellToolCall"].(map[string]any); ok {
				wantShell = c["result"].(map[string]any)["success"].(map[string]any)
			}
		}
	}
	require.NotNil(t, wantNote)
	require.NotNil(t, wantShell)
	assert.Equal(t, fmt.Sprint(wantShell["shellId"]), wantNote["task_id"], "the notification repeats the shellId")

	scratch := t.TempDir()
	call := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"sleep 1; echo BGDONE","description":"Background sleep then echo","block_until_ms":0}}]}}`
	say := func(s string) string {
		return `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"` + s + `"}]}}`
	}
	require.NoError(t, os.WriteFile(filepath.Join(scratch, "call.json"), []byte(call+"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(scratch, "launched.json"), []byte(say("LAUNCHED")+"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(scratch, "told.json"), []byte(say("FINISHED")+"\n"), 0o644))
	script := filepath.Join(scratch, "scenario.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
if grep -q 'Briefly inform the user about the task result' "$A10N_MOCK_SESSION_FILE" 2>/dev/null; then cat `+scratch+`/told.json
elif grep -q '"type":"tool_use"' "$A10N_MOCK_SESSION_FILE" 2>/dev/null; then cat `+scratch+`/launched.json
else cat `+scratch+`/call.json; fi
`), 0o755))
	ws, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	cmd := exec.Command(binary, "-p", "--force", "--output-format", "stream-json", "--script", script, "go")
	cmd.Dir, cmd.Env = ws, []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir()}
	out, err := cmd.Output()
	require.NoError(t, err, string(out))

	var shell map[string]any
	var notes []map[string]any
	var kinds []string
	for _, l := range strings.Split(string(out), "\n") {
		var f map[string]any
		if json.Unmarshal([]byte(l), &f) != nil {
			continue
		}
		if f["type"] == "system" && f["subtype"] != "init" {
			assert.Equal(t, "task_notification", f["subtype"], "no other task frame")
			notes = append(notes, f)
		}
		if f["type"] == "tool_call" && f["subtype"] == "completed" {
			shell = f["tool_call"].(map[string]any)["shellToolCall"].(map[string]any)["result"].(map[string]any)
		}
		kinds = append(kinds, fmt.Sprint(f["type"], "/", f["subtype"]))
	}
	require.NotNil(t, shell)
	assert.Equal(t, true, shell["isBackground"], "the command's call ends at once, as a background one")
	require.Len(t, notes, 1)
	assert.Equal(t, fmt.Sprint(shell["success"].(map[string]any)["shellId"]), notes[0]["task_id"])
	assert.Equal(t, wantNote["status"], notes[0]["status"])
	assert.Equal(t, "Background sleep then echo", notes[0]["title"])
	// the notification follows the call's end
	var order []string
	for _, k := range kinds {
		if k == "tool_call/completed" || k == "system/task_notification" {
			order = append(order, k)
		}
	}
	assert.Equal(t, []string{"tool_call/completed", "system/task_notification"}, order)
}
