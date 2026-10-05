package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// postToolUses are the postToolUse payloads' tool, input and output, in order.
func postToolUses(o observed) (out []map[string]any) {
	for _, h := range o.hooks {
		if h["hook_event_name"] == "postToolUse" {
			out = append(out, map[string]any{"tool": h["tool_name"], "input": h["tool_input"], "output": h["tool_output"]})
		}
	}
	return out
}

// The recorded runs runs/file-tools and runs/tool-failure: a write, reads of
// the file, a command that exits 0.

// postToolUse carries each call's tool_input and its tool_output exactly as
// recorded: the JSON of the structured result, a write's
// {"file_path":...,"success":true}, a read's {"file_path":...,"content_length":N}
// and a command's {"output":...,"exitCode":0}.
// sr:proves posttooluse-payload/cursor
func TestPostToolUseCarriesTheRecordedInputAndOutputOfEachCall(t *testing.T) {
	for _, run := range []string{"file-tools", "tool-failure"} {
		got, want := replay(t, run)
		wantPosts := postToolUses(want)
		require.NotEmpty(t, wantPosts, run)
		require.Equal(t, wantPosts, postToolUses(got), run)
	}

	got, _ := replay(t, "file-tools")
	posts := postToolUses(got)
	require.Equal(t, "Write", posts[0]["tool"])
	require.Equal(t, `{"file_path":"<RUN>/note.txt","success":true}`, posts[0]["output"])
	require.Equal(t, "Read", posts[1]["tool"])
	require.Equal(t, `{"file_path":"<RUN>/note.txt","content_length":3}`, posts[1]["output"])
	shell, _ := replay(t, "tool-failure")
	last := postToolUses(shell)
	require.Equal(t, `{"output":"FINE\n","exitCode":0}`, last[len(last)-1]["output"])
}

// postToolUse names each call (tool_use_id) and
// says how long it took (duration, in milliseconds), and fires only for a call that ran: the call a preToolUse
// hook refused fires postToolUseFailure instead (runs/file-tools,
// runs/pretool-refusal).
// sr:proves posttooluse-payload/cursor
func TestPostToolUseNamesTheCallAndItsDurationAndFiresOnlyForCallsThatRan(t *testing.T) {
	got, want := replay(t, "file-tools")
	n := 0
	for _, h := range got.raw {
		if h["hook_event_name"] != "postToolUse" {
			continue
		}
		n++
		require.NotEmpty(t, h["tool_use_id"])
		d, ok := h["duration"].(float64)
		require.True(t, ok, "duration in ms: %v", h)
		require.Greater(t, d, 0.0, "a file tool's call took some time, as recorded (0.759 to 45.586 ms): %v", h)
		require.Less(t, d, 1000.0, "a file tool's call takes milliseconds: %v", h)
	}
	for _, h := range want.raw {
		if h["hook_event_name"] == "postToolUse" {
			require.Greater(t, h["duration"].(float64), 0.0, "recorded: every file tool's call has a duration above 0")
		}
	}
	require.Equal(t, 4, n)
	require.Equal(t, len(postToolUses(want)), n)

	// a command takes time, which the hook reports
	shell, _ := replay(t, "tool-failure")
	took := 0.0
	for _, h := range shell.raw {
		if h["hook_event_name"] == "postToolUse" {
			took = h["duration"].(float64)
		}
	}
	require.Greater(t, took, 0.0, "the command that ran took some time")

	refused, rwant := replay(t, "pretool-refusal")
	require.Equal(t, postToolUses(rwant), postToolUses(refused))
	var commands []any
	for _, p := range postToolUses(refused) {
		commands = append(commands, p["input"].(map[string]any)["command"])
	}
	require.Equal(t, []any{"echo FINE"}, commands, "only the call no hook refused")
}
