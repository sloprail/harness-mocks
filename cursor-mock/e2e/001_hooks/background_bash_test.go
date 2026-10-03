package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The recorded run runs/background-bash-start: a shell command started in the
// background (sleep, then write bg.out), then an ls while it runs. The replay
// shortens the sleep; the rest is as recorded.

// withoutBackgroundResult is a hook payload without the ids a background
// command's postToolUse reports (they differ in every run).
func withoutBackgroundResult(h map[string]any) map[string]any {
	// the recorded sleep is shortened in the replay
	var out map[string]any
	b, _ := json.Marshal(h)
	_ = json.Unmarshal([]byte(strings.ReplaceAll(string(b), "sleep 25", "sleep 2")), &out)
	if s, _ := h["tool_output"].(string); h["hook_event_name"] == "postToolUse" && strings.Contains(s, "shell_id") {
		delete(out, "tool_output")
	}
	return out
}

// A shell command started in the background is answered at once with a
// receipt: its call succeeds (isBackground) with the shell id and pid and the
// terminals folder its output file lives in, and postToolUse is told
// {shell_id, pid}; no afterShellExecution fires for it. It keeps running while
// the agent works (the next command runs and does not see its result), and the
// run waits for it before its result, with a task_notification for it on the
// stream (recorded: runs/background-bash-start).
// sr:proves background-bash/cursor
func TestABackgroundShellCommandIsAnsweredAtOnceAndKeepsRunning(t *testing.T) {
	got, want := replayWith(t, "background-bash-start", func(c string) string {
		return strings.ReplaceAll(c, "sleep 25", "sleep 2")
	})
	require.Equal(t, want.frames, got.frames, "tool-call frames")
	require.Equal(t, hookNames(want), hookNames(got))
	for i := range want.hooks {
		assert.Equal(t, withoutBackgroundResult(want.hooks[i]), withoutBackgroundResult(got.hooks[i]), "hook payload %d", i)
	}
	const cmd = "sh -c 'sleep 2; echo BG-FINISHED > bg.out'"
	assert.Equal(t, "preToolUse beforeShellExecution postToolUse", joined(eventsOf(got, cmd)), "no afterShellExecution for it")

	// the receipt: the call's completed frame, and the postToolUse's output
	var receipt map[string]any
	var shellID, terminals string
	for _, f := range got.stream {
		calls, _ := f["tool_call"].(map[string]any)
		tc, _ := calls["shellToolCall"].(map[string]any)
		res, _ := tc["result"].(map[string]any)
		if f["subtype"] == "completed" && res["isBackground"] == true {
			receipt = res["success"].(map[string]any)
			terminals, _ = res["terminalsFolder"].(string)
		}
	}
	require.NotNil(t, receipt)
	assert.EqualValues(t, 0, receipt["exitCode"])
	assert.Equal(t, "SHELL_BACKGROUND_REASON_USER_REQUEST", receipt["backgroundReason"])
	shellID = strconv.Itoa(int(receipt["shellId"].(float64)))
	assert.NotZero(t, receipt["pid"])
	for _, h := range got.raw {
		if h["hook_event_name"] == "postToolUse" && commandOf(h) == cmd {
			assert.JSONEq(t, `{"shell_id":`+shellID+`,"pid":`+strconv.Itoa(int(receipt["pid"].(float64)))+`}`, h["tool_output"].(string))
		}
	}
	// the receipt names the file its output goes to, under the project's terminals folder
	assert.Equal(t, filepath.Join(got.home, ".cursor", "projects"), filepath.Dir(filepath.Dir(terminals)))
	assert.FileExists(t, filepath.Join(terminals, shellID+".txt"))

	// it kept running while the agent worked: the ls after it saw no bg.out,
	// and the command ended later, before the run's result
	for _, h := range got.raw {
		if h["hook_event_name"] == "afterShellExecution" && h["command"] == "ls" {
			assert.Equal(t, "", h["output"])
		}
	}
	b, err := os.ReadFile(filepath.Join(got.ws, "bg.out"))
	require.NoError(t, err)
	assert.Equal(t, "BG-FINISHED\n", string(b))

	// its end is on the stream, last before the result
	n := len(got.stream)
	note := got.stream[n-2]
	assert.Equal(t, "system", note["type"])
	assert.Equal(t, "task_notification", note["subtype"])
	assert.Equal(t, shellID, note["task_id"])
	assert.Equal(t, "success", note["status"])
	assert.Equal(t, "result", got.stream[n-1]["type"])
}
