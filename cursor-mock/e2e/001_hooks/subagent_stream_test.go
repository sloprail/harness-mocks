package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// subagentRun runs the mock on a recorded run's hook setup, its main agent making
// one Task call whose sub-agent plays subScript, then says DONE (taskThenDone).
// It returns the stream's frames and the hooks' payloads.
func subagentRun(t *testing.T, setupRun, subScript string) (frames, payloads []map[string]any) {
	t.Helper()
	ws, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	scratch, home := t.TempDir(), t.TempDir()
	setup, _, _, _ := recording(t, setupRun)
	copyFile(t, filepath.Join(setup, "hooks.json"), filepath.Join(ws, ".cursor", "hooks.json"), 0o644)
	copyFile(t, filepath.Join(setup, "hook.sh"), filepath.Join(ws, ".cursor", "hooks", "hook.sh"), 0o755)
	sub, script := filepath.Join(scratch, "sub.sh"), filepath.Join(scratch, "scenario.sh")
	require.NoError(t, os.WriteFile(sub, []byte(subScript), 0o755))
	require.NoError(t, os.WriteFile(script, []byte(strings.ReplaceAll(taskThenDone, "SUBSCRIPT", sub)), 0o755))
	log := filepath.Join(scratch, "payloads.jsonl")
	cmd := exec.Command(binary, "-p", "--force", "--trust", "--output-format", "stream-json", "--script", script, "go")
	cmd.Dir, cmd.Env = ws, []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "HOOK_LOG=" + log}
	out, err := cmd.Output()
	require.NoError(t, err, string(out))
	return jsonLinesOf(string(out)), readJSONL(t, log)
}

// taskCall is the taskToolCall body of a frame, if it is one.
func taskCall(f map[string]any) map[string]any {
	call, _ := f["tool_call"].(map[string]any)
	tc, _ := call["taskToolCall"].(map[string]any)
	return tc
}

// A sub-agent is announced by its Task call's frames alone: the call started,
// then completed with the sub-agent's final reply as the result's one
// assistant message and its agent id, not in the background. It has no frame of
// its own: no task_started, task_updated or task_notification
// (runs/task-stream-frames).
// sr:proves task-stream-frames/cursor
func TestASubAgentIsAnnouncedByItsTaskCallFramesAndHasNoFrameOfItsOwn(t *testing.T) {
	// the recording: the sub-agent's two Task frames, and no frame of a task of the sub-agent
	var started, completed map[string]any
	for _, f := range recordedStream(t, "task-stream-frames") {
		if f["type"] == "system" && f["subtype"] == "task_notification" {
			assert.NotEqual(t, taskCall(completed)["result"].(map[string]any)["success"].(map[string]any)["agentId"], f["task_id"], "the notification is the background command's")
		}
		if tc := taskCall(f); tc != nil {
			if f["subtype"] == "started" {
				started = tc
			} else {
				completed = f
			}
		}
	}
	require.NotNil(t, started)
	require.NotNil(t, completed)
	want := taskCall(completed)["result"].(map[string]any)["success"].(map[string]any)
	assert.Equal(t, "PONG", want["conversationSteps"].([]any)[0].(map[string]any)["assistantMessage"].(map[string]any)["text"])

	frames, _ := subagentRun(t, "task-stream-frames", pongScript)
	var seen []string
	var got map[string]any
	for _, f := range frames {
		if f["type"] == "system" && f["subtype"] != "init" {
			t.Errorf("a frame of its own for the sub-agent: %v", f)
		}
		if tc := taskCall(f); tc != nil {
			seen = append(seen, fmt.Sprint(f["subtype"]))
			if f["subtype"] == "completed" {
				got = tc["result"].(map[string]any)["success"].(map[string]any)
			}
		}
	}
	assert.Equal(t, []string{"started", "completed"}, seen)
	require.NotNil(t, got)
	assert.Equal(t, want["conversationSteps"], got["conversationSteps"], "the sub-agent's final reply is the call's result")
	assert.Equal(t, false, got["isBackground"])
	assert.NotEmpty(t, got["agentId"])
}

// The payload of an event raised inside a sub-agent carries the sub-agent's own
// conversation and session id, not the main agent's, and no field that names
// the sub-agent or its parent: the fields are those of the main agent's events
// (runs/subagent-lifecycle-hooks).
// sr:proves hook-common-payload/cursor
func TestASubAgentsEventsCarryItsOwnSessionIdAndNoFieldNamingIt(t *testing.T) {
	check := func(name string, payloads []map[string]any) {
		var main string
		for _, p := range payloads {
			if p["hook_event_name"] == "sessionStart" {
				main = p["session_id"].(string)
			}
		}
		require.NotEmpty(t, main, name)
		inside := 0
		for _, p := range payloads {
			if p["hook_event_name"] == "afterAgentThought" || p["hook_event_name"] == "BackgroundTick" {
				continue
			}
			assert.Equal(t, p["session_id"], p["conversation_id"], "%s: %v", name, p["hook_event_name"])
			for k := range p {
				assert.NotContains(t, strings.ToLower(k), "subagent", "%s: %s", name, k)
				assert.NotContains(t, strings.ToLower(k), "parent", "%s: %s", name, k)
			}
			if p["session_id"] != main {
				inside++
			}
		}
		assert.Positive(t, inside, "%s: an event raised inside the sub-agent", name)
	}
	check("recording", recordedRaw(t, "subagent-lifecycle-hooks"))
	_, got := subagentRun(t, "subagent-lifecycle-hooks", layerScript(""))
	check("mock", got)
}
