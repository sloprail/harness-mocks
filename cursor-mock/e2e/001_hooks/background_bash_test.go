package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// The recorded run runs/background-bash-start: a shell command started in the
// background (sleep, then write bg.out), then an ls while it runs.

// A shell command started in the background is answered at once with a
// receipt: its call succeeds (isBackground) with the shell id, the pid and the
// terminals folder its output file <shell id>.txt lives in, and postToolUse is
// told {shell_id, pid}; no afterShellExecution fires for it. It keeps running
// while the agent works (the ls after it ran and the command ended later), and
// its end is a task_notification on the stream before the result (recorded:
// runs/background-bash-start).
// sr:proves background-bash/cursor
func TestABackgroundShellCommandIsAnsweredAtOnceAndKeepsRunning(t *testing.T) {
	t.Parallel()
	got := replayBackground(t, "background-bash-start")
	want := recordedStream(t, "background-bash-start")
	require.Equal(t, bgKinds(want), bgKinds(got.frames), "the recorded order: two calls, the notification, the result")

	// the receipt: the call's completed frame, and the postToolUse's output
	var receipt map[string]any
	var terminals string
	for _, f := range got.frames {
		calls, _ := f["tool_call"].(map[string]any)
		tc, _ := calls["shellToolCall"].(map[string]any)
		res, _ := tc["result"].(map[string]any)
		if f["subtype"] == "completed" && res["isBackground"] == true {
			receipt, _ = res["success"].(map[string]any)
			terminals, _ = res["terminalsFolder"].(string)
		}
	}
	require.NotNil(t, receipt)
	require.EqualValues(t, 0, receipt["exitCode"])
	require.Equal(t, "SHELL_BACKGROUND_REASON_USER_REQUEST", receipt["backgroundReason"])
	id := itoa(int(receipt["shellId"].(float64)))
	require.NotZero(t, receipt["pid"])
	require.FileExists(t, filepath.Join(terminals, id+".txt"), "the file its output goes to")

	const cmd = "sh -c 'sleep 25; echo BG-FINISHED > bg.out'"
	var tool map[string]any
	var events []string
	for _, h := range got.hooks {
		if commandOf(h) == cmd || h["command"] == cmd {
			events = append(events, h["hook_event_name"].(string))
		}
		if h["hook_event_name"] == "afterShellExecution" {
			require.NotEqual(t, cmd, h["command"], "no end of its own to report")
		}
		if h["hook_event_name"] == "postToolUse" && commandOf(h) == cmd {
			require.NoError(t, json.Unmarshal([]byte(h["tool_output"].(string)), &tool))
		}
	}
	require.Equal(t, []string{"preToolUse", "beforeShellExecution", "postToolUse"}, events, "the hooks around it, and no afterShellExecution")
	require.EqualValues(t, receipt["shellId"], tool["shell_id"])
	require.EqualValues(t, receipt["pid"], tool["pid"])

	// it ran on while the agent worked: the ls came after it, and the
	// notification names the command's end
	require.Equal(t, id, notificationOf(t, got.frames)["task_id"])

	// the command really ran to its end: it wrote bg.out in the workspace
	roots, _ := got.hooks[0]["workspace_roots"].([]any)
	require.NotEmpty(t, roots)
	b, err := os.ReadFile(filepath.Join(roots[0].(string), "bg.out"))
	require.NoError(t, err)
	require.Equal(t, "BG-FINISHED\n", string(b))
}
