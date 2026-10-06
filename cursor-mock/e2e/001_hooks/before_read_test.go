package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
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
// sr:proves pretooluse-refusal/cursor
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

// readErrors are the error messages of the read calls' completed frames in a
// stream (the text the agent is given for a blocked read), in order.
func readErrors(stream, ws string) (out []string) {
	for _, l := range strings.Split(stream, "\n") {
		var f struct {
			Subtype  string
			ToolCall struct {
				Read struct {
					Result struct{ Error struct{ ErrorMessage string } }
				} `json:"readToolCall"`
			} `json:"tool_call"`
		}
		if json.Unmarshal([]byte(l), &f) == nil && f.Subtype == "completed" && f.ToolCall.Read.Result.Error.ErrorMessage != "" {
			msg := f.ToolCall.Read.Result.Error.ErrorMessage
			if ws != "" {
				msg = strings.ReplaceAll(msg, ws, "<RUN>")
			}
			out = append(out, msg)
		}
	}
	return out
}

// TestABlockedReadTellsTheAgentWhatTheFailureHookIsTold: recorded, the text the
// agent gets for a blocked read (the error of the read's completed frame) is
// the failure hook's error_message, word for word, for each way of blocking.
// sr:proves file-tools/cursor
func TestABlockedReadTellsTheAgentWhatTheFailureHookIsTold(t *testing.T) {
	got, want := replay(t, "before-read-refusal")
	recorded, err := os.ReadFile(filepath.Join(newestSample(t, "before-read-refusal"), "stream.jsonl"))
	require.NoError(t, err)
	var hooked []string
	for _, h := range want.hooks {
		if h["hook_event_name"] == "postToolUseFailure" {
			hooked = append(hooked, h["error_message"].(string))
		}
	}
	require.Len(t, hooked, 5)
	require.Equal(t, hooked, readErrors(string(recorded), ""), "recorded")
	require.Equal(t, hooked, readErrors(got.stdout, got.ws), "mock")
}

func completedReads(frames []string) (out []string) {
	for _, f := range frames {
		if strings.HasPrefix(f, "tool_call/completed/readToolCall/") {
			out = append(out, f)
		}
	}
	return out
}

// TestABeforeReadFileHookThatAllowsByJSONLetsTheReadProceed: the doc says a JSON
// permission of "allow" lets the read go on; recorded (runs/before-read-refusal),
// the failClosed hook answers {"permission":"allow"} to every file but e.txt, and
// d.txt, which the other hook left alone by a crash, is read. The mock's read
// of a file whose hook allows by JSON has its postToolUse and no failure.
// sr:proves file-tools/cursor
func TestABeforeReadFileHookThatAllowsByJSONLetsTheReadProceed(t *testing.T) {
	_, want := replay(t, "before-read-refusal")
	var allowedRecorded bool
	for _, h := range want.hooks {
		if h["hook_event_name"] == "postToolUse" {
			in, _ := h["tool_input"].(map[string]any)
			allowedRecorded = allowedRecorded || in["file_path"] == "<RUN>/d.txt"
		}
	}
	require.True(t, allowedRecorded, "recorded: d.txt, which the allowing hook did not stop, was read")

	r := runTools(t, `{"version":1,"hooks":{"beforeReadFile":[{"command":".cursor/hooks/allow.sh"}]}}`,
		map[string]string{"allow.sh": "#!/bin/sh\ncat >/dev/null\necho '{\"permission\":\"allow\"}'\n"},
		map[string]string{"note.txt": "hi\n"},
		map[string]any{"name": "Read", "input": map[string]any{"file_path": "note.txt"}})
	var ok bool
	for _, f := range r.frames {
		if tc, _ := f["tool_call"].(map[string]any); tc != nil && f["subtype"] == "completed" {
			if body, _ := tc["readToolCall"].(map[string]any); body != nil {
				res, _ := body["result"].(map[string]any)
				_, ok = res["success"]
			}
		}
	}
	require.True(t, ok, "the read succeeded")
}
