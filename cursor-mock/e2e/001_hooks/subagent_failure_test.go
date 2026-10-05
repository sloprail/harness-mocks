package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The recorded run runs/foreground-subagent-failure: a foreground Task call
// whose model parameter is the invalid name no-such-model-xyz.

// taskErrors are the errors of the Task call's completed frames in a stream,
// and whether any Task call was shown as started.
func taskErrors(frames []map[string]any) (errs []string, started bool) {
	for _, f := range frames {
		tc, _ := f["tool_call"].(map[string]any)
		body, _ := tc["taskToolCall"].(map[string]any)
		if body == nil {
			continue
		}
		started = started || f["subtype"] == "started"
		if r, _ := body["result"].(map[string]any); r != nil {
			if e, _ := r["error"].(map[string]any); e != nil {
				errs = append(errs, e["error"].(string))
			}
		}
	}
	return errs, started
}

// taskHooks are the events of the hooks that named the Task tool.
func taskHooks(hooks []map[string]any) (out []string) {
	for _, h := range hooks {
		if h["tool_name"] == "Task" {
			out = append(out, h["hook_event_name"].(string))
		}
	}
	return out
}

// TestATaskCallWithAnInvalidModelFailsWithoutStartingASubAgent: recorded, a
// Task call naming a model that cannot be resolved is not started: its
// preToolUse hooks fire, and it completes at once with an error that names the
// model and says it "could not be resolved to a valid subagent model" (then
// lists the account's models), with no postToolUse or postToolUseFailure and no
// sub-agent. The mock answers the same, naming the model, and lists only the
// model it has (the account's catalogue is not modeled).
// sr:proves foreground-subagent-result/cursor
func TestATaskCallWithAnInvalidModelFailsWithoutStartingASubAgent(t *testing.T) {
	setup, _, _, _ := recording(t, "foreground-subagent-failure")
	sample := newestSample(t, "foreground-subagent-failure")
	want, wantStarted := taskErrors(readJSONL(t, filepath.Join(sample, "stream.jsonl")))
	require.Len(t, want, 1)
	require.False(t, wantStarted, "recorded: no started frame")
	require.Equal(t, []string{"preToolUse"}, taskHooks(readJSONL(t, filepath.Join(sample, "payloads.jsonl"))))
	const head = "Invalid model selection \"no-such-model-xyz\". Model could not be resolved to a valid subagent model.\nAllowed model slugs:\n"
	require.True(t, strings.HasPrefix(want[0], head), want[0])

	ws, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	scratch, home := t.TempDir(), t.TempDir()
	copyFile(t, filepath.Join(setup, "hooks.json"), filepath.Join(ws, ".cursor", "hooks.json"), 0o644)
	copyFile(t, filepath.Join(setup, "hook.sh"), filepath.Join(ws, ".cursor", "hooks", "hook.sh"), 0o755)
	script := filepath.Join(scratch, "scenario.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
n=$(grep -c '"type":"tool_use"' "$A10N_MOCK_SESSION_FILE" 2>/dev/null)
if [ "${n:-0}" = 0 ]; then
  printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"call_1","name":"Task","input":{"description":"d","prompt":"reply PINEAPPLE-7","subagent_type":"generalPurpose","model":"no-such-model-xyz","script":"/nonexistent"}}]}}'
else
  printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"result":"DONE"}'
fi
`), 0o755))
	logPath := filepath.Join(scratch, "payloads.jsonl")
	cmd := exec.Command(binary, "-p", "--force", "--trust", "--output-format", "stream-json", "--script", script, "go")
	cmd.Dir, cmd.Env = ws, []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "HOOK_LOG=" + logPath}
	out, err := cmd.Output()
	require.NoError(t, err, string(out))
	got, started := taskErrors(readJSONLText(t, string(out)))
	require.Len(t, got, 1)
	require.False(t, started, "the mock shows no started frame either")
	require.True(t, strings.HasPrefix(got[0], head), got[0])
	require.Equal(t, []string{"preToolUse"}, taskHooks(readJSONL(t, logPath)))
}
