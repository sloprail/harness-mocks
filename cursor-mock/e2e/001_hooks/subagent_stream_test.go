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
		if f["type"] == "system" && f["subtype"] != "init" && f["subtype"] != "task_notification" {
			t.Errorf("a frame of its own for a sub-agent in the recording: %v", f)
		}
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

	assert.Equal(t, false, want["isBackground"])
	assert.NotEmpty(t, want["agentId"])

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
// the sub-agent or its parent: the fields are those of the main agent's events.
// Once its transcript exists its transcript_path is the sub-agent's own, not the
// main agent's, and its Shell calls carry cwd and the project's workspace_roots
// as the main agent's do (runs/subagent-lifecycle-hooks).
// sr:proves hook-common-payload/cursor
func TestASubAgentsEventsCarryItsOwnSessionIdAndNoFieldNamingIt(t *testing.T) {
	check := func(name string, payloads []map[string]any) {
		var main string
		var mainRoots any
		for _, p := range payloads {
			if p["hook_event_name"] == "sessionStart" {
				main, mainRoots = p["session_id"].(string), p["workspace_roots"]
			}
		}
		require.NotEmpty(t, main, name)
		inside, own := 0, 0
		for _, p := range payloads {
			if p["hook_event_name"] == "afterAgentThought" || p["hook_event_name"] == "BackgroundTick" {
				continue
			}
			assert.NotContains(t, []any{"subagentStart", "subagentStop"}, p["hook_event_name"], "%s: neither fires for the Task call in print mode", name)
			if p["tool_name"] != "Shell" {
				assert.NotContains(t, p, "cwd", "%s: %v of %v carries no cwd, as the Task call's preToolUse does not", name, p["hook_event_name"], p["tool_name"])
			}
			assert.Equal(t, p["session_id"], p["conversation_id"], "%s: %v", name, p["hook_event_name"])
			for k := range p {
				assert.NotContains(t, strings.ToLower(k), "subagent", "%s: %s", name, k)
				assert.NotContains(t, strings.ToLower(k), "parent", "%s: %s", name, k)
			}
			if p["session_id"] != main {
				inside++
				assert.Equal(t, mainRoots, p["workspace_roots"], "%s: the sub-agent's event names the same project roots", name)
				if p["tool_name"] == "Shell" {
					assert.Equal(t, "", p["cwd"], "%s: a Shell call of the sub-agent carries cwd, recorded empty", name)
				}
				if tp, ok := p["transcript_path"].(string); ok {
					own++
					sid := p["session_id"].(string)
					assert.True(t, strings.HasSuffix(tp, "/agent-transcripts/"+sid+"/"+sid+".jsonl"), "%s: its own transcript, not the main agent's: %s", name, tp)
				}
			}
		}
		assert.Positive(t, inside, "%s: an event raised inside the sub-agent", name)
		assert.Positive(t, own, "%s: an event of the sub-agent carries its own transcript path", name)
	}
	check("recording", recordedRaw(t, "subagent-lifecycle-hooks"))
	_, got := subagentRun(t, "subagent-lifecycle-hooks", layerScript(""))
	check("mock", got)
}

// shellCall is the shellToolCall body of a frame, if it is one.
func shellCall(f map[string]any) map[string]any {
	call, _ := f["tool_call"].(map[string]any)
	sc, _ := call["shellToolCall"].(map[string]any)
	return sc
}

// bgStreamOf describes how a stream announces a command started in the
// background: its call's started and completed frames, the completed one's
// receipt (background, with the shell id), and the one task_notification
// naming that shell id with its status and title, after the call's end.
type bgStreamOf struct {
	started, completed, receipt, args, notification map[string]any
	order                                           []string
}

func bgStream(t *testing.T, name string, frames []map[string]any) bgStreamOf {
	t.Helper()
	var s bgStreamOf
	for _, f := range frames {
		if sc := shellCall(f); sc != nil {
			s.order = append(s.order, "shell/"+fmt.Sprint(f["subtype"]))
			if f["subtype"] == "started" {
				s.started, s.args = sc, sc["args"].(map[string]any)
			} else {
				s.completed = sc
				res := sc["result"].(map[string]any)
				assert.Equal(t, true, res["isBackground"], name)
				s.receipt = res["success"].(map[string]any)
			}
		}
		if f["type"] == "system" && f["subtype"] == "task_notification" {
			s.order = append(s.order, "notification")
			s.notification = f
		}
		if taskCall(f) != nil {
			s.order = append(s.order, "task/"+fmt.Sprint(f["subtype"]))
		}
	}
	require.NotNil(t, s.started, name)
	require.NotNil(t, s.completed, name)
	require.NotNil(t, s.notification, name)
	return s
}

// A command started in the background is announced the same way as a sub-agent
// is: by the frames of the call that launched it (started, then completed at
// once as a background call with its shell id) and, when it ends, by the one
// task_notification frame naming that shell id, its status and its title (the
// call's description), after the call's end. No task_started or task_updated
// frame exists (runs/task-stream-frames).
// sr:proves task-stream-frames/cursor
func TestABackgroundCommandIsAnnouncedByItsCallFramesAndOneNotificationNamingItsShellId(t *testing.T) {
	rec := bgStream(t, "recording", recordedStream(t, "task-stream-frames"))
	assert.Equal(t, true, rec.args["isBackground"])
	assert.Equal(t, "notification", rec.order[len(rec.order)-1], "the notification comes after the calls' ends")
	assert.EqualValues(t, rec.notification["task_id"], fmt.Sprint(int64(rec.receipt["shellId"].(float64))))
	assert.Equal(t, "success", rec.notification["status"])
	assert.Equal(t, rec.args["description"], rec.notification["title"])
	assert.Equal(t, "SHELL_BACKGROUND_REASON_USER_REQUEST", rec.receipt["backgroundReason"])

	ws, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	scratch, home := t.TempDir(), t.TempDir()
	sub, script := filepath.Join(scratch, "sub.sh"), filepath.Join(scratch, "scenario.sh")
	require.NoError(t, os.WriteFile(sub, []byte(pongScript), 0o755))
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
n=$(grep -c '"type":"tool_use"' "$A10N_MOCK_SESSION_FILE" 2>/dev/null)
case "${n:-0}" in
0) printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu_0","name":"Bash","input":{"command":"sh -c '"'"'sleep 1; echo BGDONE'"'"'","block_until_ms":0,"description":"Background sleep then echo"}}]}}' ;;
1) printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu_1","name":"Task","input":{"description":"Reply with PONG","prompt":"Reply with the single word PONG.","subagent_type":"generalPurpose","script":"`+sub+`"}}]}}' ;;
*) printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"DONE"}]}}' '{"type":"result","subtype":"success","result":"DONE"}' ;;
esac
`), 0o755))
	cmd := exec.Command(binary, "-p", "--force", "--trust", "--output-format", "stream-json", "--script", script, "go")
	cmd.Dir, cmd.Env = ws, []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "TMPDIR=" + scratch}
	out, err := cmd.Output()
	require.NoError(t, err, string(out))
	got := bgStream(t, "mock", jsonLinesOf(string(out)))

	assert.Equal(t, true, got.args["isBackground"])
	assert.Equal(t, rec.args["description"], got.args["description"])
	assert.Equal(t, rec.receipt["backgroundReason"], got.receipt["backgroundReason"])
	assert.NotZero(t, got.receipt["shellId"])
	assert.Equal(t, fmt.Sprint(int64(got.receipt["shellId"].(float64))), got.notification["task_id"])
	assert.Equal(t, rec.notification["status"], got.notification["status"])
	assert.Equal(t, got.args["description"], got.notification["title"])
	assert.Equal(t, []string{"shell/started", "shell/completed", "task/started", "task/completed", "notification"}, got.order, "the notification follows the calls that launched the tasks")
}
