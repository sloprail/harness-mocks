package e2e

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The recorded run runs/before-read-timeout: three Reads of existing files with
// two beforeReadFile hooks of a 1 s timeout that run for 5 s on one file each:
// the first, on a.txt, as it is; the second, on b.txt, failClosed. c.txt is
// read with neither hook slow.

// TestABeforeReadFileHookThatTimesOutLetsTheReadThroughUnlessItIsFailClosed:
// recorded, a beforeReadFile hook that outruns its timeout is killed before it
// finishes and the read goes through (it has a postToolUse); a failClosed one
// blocks the read as an error result and a postToolUseFailure that says the
// hook failed closed and timed out after its limit, with no postToolUse; the
// read neither slow hook held up is untouched.
// sr:proves hook-timeout/cursor
// sr:proves file-tools/cursor
func TestABeforeReadFileHookThatTimesOutLetsTheReadThroughUnlessItIsFailClosed(t *testing.T) {
	got, want := replay(t, "before-read-timeout")
	conforms(t, got, want)

	for name, o := range map[string]observed{"recorded": want, "mock": got} {
		var passed, blocked []string
		var message string
		for _, h := range o.hooks {
			in, _ := h["tool_input"].(map[string]any)
			switch h["hook_event_name"] {
			case "postToolUse":
				passed = append(passed, in["file_path"].(string))
			case "postToolUseFailure":
				blocked = append(blocked, in["file_path"].(string))
				message, _ = h["error_message"].(string)
			}
		}
		require.Equal(t, []string{"<RUN>/a.txt", "<RUN>/c.txt"}, passed, name+": a timeout fails open, so a.txt and the untouched c.txt are read")
		require.Equal(t, []string{"<RUN>/b.txt"}, blocked, name+": only the failClosed hook's timeout blocks the read")
		require.Contains(t, message, "File read was blocked by a hook: Tool blocked because this hook is configured to fail closed", name)
		require.Contains(t, message, `Hook ".cursor/hooks/slow-read.sh b.txt" execution failed: Hook script timed out after 1000ms`, name)
		for _, r := range o.results {
			require.False(t, strings.HasSuffix(r, ":finished"), name+": a hook that outran its timeout was killed before it finished")
		}
	}
	require.Equal(t, []string{"tool_call/completed/readToolCall/success", "tool_call/completed/readToolCall/error", "tool_call/completed/readToolCall/success"}, completedReads(want.frames))
	require.Equal(t, completedReads(want.frames), completedReads(got.frames))
}
