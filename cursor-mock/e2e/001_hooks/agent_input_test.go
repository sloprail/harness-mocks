package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The recorded runs runs/agent-input-validation (a Task call with an empty
// argument object) and runs/agent-input-validation-description (one with a
// prompt only). The hooks.json and hook script are the runs' own.

// dispatches plays the given Task inputs (one tool call each, in order)
// against the mock on the recorded setup, and returns the stream's frames and
// the payloads the hooks read.
func dispatches(t *testing.T, setup string, inputs ...string) (frames, hooks []map[string]any) {
	t.Helper()
	ws, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	scratch := t.TempDir()
	copyFile(t, filepath.Join(setup, "hooks.json"), filepath.Join(ws, ".cursor", "hooks.json"), 0o644)
	copyFile(t, filepath.Join(setup, "hook.sh"), filepath.Join(ws, ".cursor", "hooks", "hook.sh"), 0o755)
	var sh strings.Builder
	sh.WriteString("#!/bin/sh\nn=$(grep -c '\"type\":\"tool_use\"' \"$A10N_MOCK_SESSION_FILE\" 2>/dev/null)\nn=${n:-0}\ncase $n in\n")
	for i, in := range inputs {
		sh.WriteString(itoa(i) + `) printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu_` +
			itoa(i) + `","name":"Task","input":` + in + `}]}}';;` + "\n")
	}
	sh.WriteString(`*) printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"result":"DONE"}';;` + "\nesac\n")
	script := filepath.Join(scratch, "scenario.sh")
	require.NoError(t, os.WriteFile(script, []byte(sh.String()), 0o755))
	logPath := filepath.Join(scratch, "payloads.jsonl")
	cmd := exec.Command(binary, "-p", "--force", "--trust", "--output-format", "stream-json", "go")
	cmd.Dir = ws
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir(), "HOOK_LOG=" + logPath, "TMPDIR=" + scratch, "A10N_MOCK_SCRIPT=" + script}
	out, err := cmd.Output()
	require.NoError(t, err, "the mock failed: %s", out)
	for _, l := range strings.Split(string(out), "\n") {
		var f map[string]any
		if json.Unmarshal([]byte(l), &f) == nil {
			frames = append(frames, f)
		}
	}
	for _, m := range readJSONL(t, logPath) {
		if m["hook_event_name"] != nil {
			hooks = append(hooks, m)
		}
	}
	return frames, hooks
}

// taskResults are the results of the stream's completed Task calls, in order.
func taskResults(frames []map[string]any) (out []map[string]any) {
	for _, f := range frames {
		tc, _ := f["tool_call"].(map[string]any)
		if body, ok := tc["taskToolCall"].(map[string]any); ok && f["subtype"] == "completed" {
			out = append(out, body["result"].(map[string]any))
		}
	}
	return out
}

// A Task call lacking its prompt is refused with "Invalid arguments:" and the
// parameter "Required"; nothing runs and no hook fires, not even preToolUse
// (recorded: runs/agent-input-validation). Its description is not required: a
// call with a prompt only passes validation and reaches preToolUse with an
// empty description (recorded: runs/agent-input-validation-description).
// sr:proves agent-input-validation/cursor
func TestTaskWithoutPromptIsRefusedBeforeAnyHook(t *testing.T) {
	root := filepath.Join("..", "..", "snapshots", "runs")
	_, rec, _, _ := recording(t, "agent-input-validation")
	for _, h := range rec.hooks {
		assert.NotContains(t, []any{"preToolUse", "postToolUse", "postToolUseFailure", "subagentStart"}, h["hook_event_name"], "the recording: no tool or sub-agent hook")
	}
	require.Equal(t, []string{"tool_call/completed/taskToolCall/error", "result/success"}, rec.frames)

	frames, hooks := dispatches(t, filepath.Join(root, "agent-input-validation", "setup"), `{}`, `{"description":"helper"}`)
	results := taskResults(frames)
	require.Len(t, results, 2)
	for _, r := range results {
		assert.Equal(t, map[string]any{"error": map[string]any{"error": "Invalid arguments:\nprompt: Required"}}, r)
	}
	for _, h := range hooks {
		assert.NotContains(t, []any{"preToolUse", "postToolUse", "postToolUseFailure"}, h["hook_event_name"], "no tool hook for a refused dispatch")
	}

	// a prompt alone is enough: the description is empty
	_, drec, _, _ := recording(t, "agent-input-validation-description")
	var recordedInput map[string]any
	for _, h := range drec.hooks {
		if h["hook_event_name"] == "preToolUse" {
			recordedInput = h["tool_input"].(map[string]any)
		}
	}
	require.Equal(t, map[string]any{"description": "", "prompt": "Reply with the word HI", "subagent_type": "generalPurpose"}, recordedInput)
	_, hooks = dispatches(t, filepath.Join(root, "agent-input-validation-description", "setup"), `{"prompt":"Reply with the word HI"}`)
	var pre []map[string]any
	for _, h := range hooks {
		if h["hook_event_name"] == "preToolUse" {
			pre = append(pre, h["tool_input"].(map[string]any))
		}
	}
	assert.Equal(t, []map[string]any{recordedInput}, pre)
}
