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
