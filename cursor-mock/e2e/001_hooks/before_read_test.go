package e2e

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The recorded run runs/before-read-refusal: six Reads of existing files with
// a beforeReadFile hook that answers a.txt by exit 2, b.txt by a JSON deny,
// c.txt by exit 0 with output that is not JSON, d.txt by exit 1 and f.txt by
// JSON with a permission that is not one, and a second, failClosed hook that
// exits 1 on e.txt.

// TestABeforeReadFileHookThatRefusesBlocksTheRead: recorded, a beforeReadFile
// hook blocks the read by exit 2, by a JSON deny, by output that is not JSON
// and by JSON with an unknown permission (worded apart), and a failClosed hook
// blocks it by exiting 1, where a hook not failClosed that exits 1 lets the
// read through (a crash fails open); the hook is told the file's
// content even for a read it blocks; a blocked read ends as an error result
// and a postToolUseFailure with permission_denied and the hook's message (a
// deny's user_message, exit 2's stderr, or the invalid-JSON notice), with no
// postToolUse; the read that is not blocked has its postToolUse.
// sr:proves file-tools/cursor
// sr:proves hook-exit-code-semantics/cursor
// sr:proves tool-failure-hook/cursor
func TestABeforeReadFileHookThatRefusesBlocksTheRead(t *testing.T) {
	got, want := replay(t, "before-read-refusal")
	conforms(t, got, want)

	const tail = "\n\nTo view or modify configured hooks, go to Cursor Settings > Hooks.\n\nAgent note: Do not suggest workarounds to the blocked tool."
	messages := map[string]string{
		"<RUN>/a.txt": "File read was blocked by a hook: Hook blocked with message: EXIT2-MSG" + tail,
		"<RUN>/b.txt": "File read was blocked by a hook: JSON-DENY-MSG" + tail,
		"<RUN>/c.txt": `File read was blocked by a hook: Hook ".cursor/hooks/hook.sh" returned invalid JSON. The command was blocked for safety.` + tail,
		"<RUN>/e.txt": `File read was blocked by a hook: Tool blocked because this hook is configured to fail closed (block when it fails). Hook ".cursor/hooks/closed.sh" failed with exit code 1: CLOSED-CRASH` + tail,
		"<RUN>/f.txt": `File read was blocked by a hook: Hook ".cursor/hooks/hook.sh" returned an invalid response for this hook step. The command was blocked for safety.` + tail,
	}
	for name, o := range map[string]observed{"recorded": want, "mock": got} {
		failed := map[string]string{}
		var succeeded, before []string
		for _, h := range o.hooks {
			in, _ := h["tool_input"].(map[string]any)
			switch h["hook_event_name"] {
			case "beforeReadFile":
				before = append(before, h["file_path"].(string))
				require.Contains(t, []any{"CONTENT-a\n", "CONTENT-b\n", "CONTENT-c\n", "CONTENT-d\n", "CONTENT-e\n", "CONTENT-f\n"}, h["content"], name)
			case "postToolUseFailure":
				failed[in["file_path"].(string)] = h["error_message"].(string)
				require.Equal(t, "permission_denied", h["failure_type"], name)
			case "postToolUse":
				succeeded = append(succeeded, in["file_path"].(string))
			}
		}
		require.Equal(t, []string{"<RUN>/a.txt", "<RUN>/b.txt", "<RUN>/c.txt", "<RUN>/d.txt", "<RUN>/e.txt", "<RUN>/f.txt"}, before, name+": every read tells the hook its file, whether or not it is blocked")
		require.Equal(t, messages, failed, name)
		require.Equal(t, []string{"<RUN>/d.txt"}, succeeded, name+": only the read exit 1 left alone has a postToolUse")
	}
	for _, o := range []observed{want, got} {
		require.Equal(t, []string{"tool_call/completed/readToolCall/error", "tool_call/completed/readToolCall/error", "tool_call/completed/readToolCall/error", "tool_call/completed/readToolCall/success", "tool_call/completed/readToolCall/error", "tool_call/completed/readToolCall/error"}, completedReads(o.frames))
	}
}

func completedReads(frames []string) (out []string) {
	for _, f := range frames {
		if strings.HasPrefix(f, "tool_call/completed/readToolCall/") {
			out = append(out, f)
		}
	}
	return out
}
