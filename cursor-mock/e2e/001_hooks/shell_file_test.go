package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The recorded run runs/shell-exit-status: commands exiting 1 with no output,
// 1 with output, 2 with both streams, and 0.

// TestAShellCommandExitingNonZeroIsAFailureResultWithItsOutputOrExitCode:
// recorded, whatever the status (a grep that finds nothing exits 1 and fails
// like any other), the tool's result is a failure, and the failure hook's
// message is what the command printed (stdout then stderr), or "Command failed
// with exit code N" when it printed nothing.
// sr:proves bash-tool-result/cursor
func TestAShellCommandExitingNonZeroIsAFailureResultWithItsOutputOrExitCode(t *testing.T) {
	got, want := replay(t, "shell-exit-status")
	conforms(t, got, want)

	for cmd, msg := range map[string]string{
		"grep zzz /dev/null":                                    "Command failed with exit code 1",
		"sh -c 'echo SOME-OUTPUT; exit 1'":                      "SOME-OUTPUT",
		"sh -c 'echo MORE-OUTPUT; echo ERR-OUTPUT >&2; exit 2'": "MORE-OUTPUT\nERR-OUTPUT",
	} {
		m, kind, ok := failureOf(got, cmd)
		require.True(t, ok, cmd)
		require.Equal(t, msg, m, cmd)
		require.Equal(t, "error", kind, cmd)
	}
	require.Equal(t, []string{"tool_call/started/shellToolCall/", "tool_call/completed/shellToolCall/failure"}, got.frames[:2])
	require.Equal(t, "tool_call/completed/shellToolCall/success", got.frames[len(got.frames)-2], "echo FINE")

	// a command that succeeds has the structured result the recording shows:
	// its output and exit code, as the postToolUse hook's tool_output
	var fine string
	for _, h := range got.raw {
		if h["hook_event_name"] == "postToolUse" && h["tool_input"].(map[string]any)["command"] == "echo FINE" {
			fine, _ = h["tool_output"].(string)
		}
	}
	require.JSONEq(t, `{"output":"FINE\n","exitCode":0}`, fine)
}

// TestAFileReadReturnsItsContentAndAWriteReplacesItWhole: recorded, a write
// creates the file with the content it was given, a later write replaces it
// whole, a read returns what the file holds, and a read of a file that is not
// there is an error result.
// sr:proves file-tools/cursor
func TestAFileReadReturnsItsContentAndAWriteReplacesItWhole(t *testing.T) {
	got, want := replay(t, "file-tools")
	conforms(t, got, want)

	var written, edits []any
	for _, h := range got.raw {
		switch {
		case h["hook_event_name"] == "preToolUse" && h["tool_name"] == "Write":
			written = append(written, h["tool_input"].(map[string]any)["content"])
		case h["hook_event_name"] == "afterFileEdit":
			edits = append(edits, h["edits"])
		}
	}
	require.Equal(t, []any{"hi\n", "bye\n"}, written)
	var recordedEdits []any
	for _, h := range recordedRaw(t, "file-tools") {
		if h["hook_event_name"] == "afterFileEdit" {
			recordedEdits = append(recordedEdits, h["edits"])
		}
	}
	require.Equal(t, []any{
		[]any{map[string]any{"old_string": "", "new_string": "hi\n"}},
		[]any{map[string]any{"old_string": "hi", "new_string": "bye"}},
	}, recordedEdits, "recorded: the edits the two writes made")
	require.Equal(t, recordedEdits, edits, "the mock reports the same edits")
	require.Contains(t, got.frames, "tool_call/completed/readToolCall/success")
	// a read's structured result is its file and the length of the content it
	// returned, as the postToolUse hook's tool_output
	var reads, beforeReads int
	for _, h := range got.raw {
		if h["hook_event_name"] == "beforeReadFile" {
			// recorded: each successful read reports the file's path and content
			beforeReads++
			require.Equal(t, got.ws+"/note.txt", h["file_path"])
			require.Equal(t, "hi\n", h["content"])
			require.Equal(t, []any{}, h["attachments"])
		}
		if h["hook_event_name"] == "postToolUse" && h["tool_name"] == "Read" {
			reads++
			require.JSONEq(t, `{"file_path":"`+got.ws+`/note.txt","content_length":3}`, h["tool_output"].(string))
		}
	}
	require.Equal(t, reads, beforeReads, "each read fires beforeReadFile")
	require.NotZero(t, reads, "a read has its result as the postToolUse hook's tool_output")
}

// TestAReadOfAFileThatIsNotThereIsAnErrorResult: recorded, the Read of a
// missing file fails with an error result, and the failure hook says so.
// sr:proves file-tools/cursor
func TestAReadOfAFileThatIsNotThereIsAnErrorResult(t *testing.T) {
	got, want := replay(t, "tool-failure")
	conforms(t, got, want)
	require.Contains(t, got.frames, "tool_call/completed/readToolCall/error")
}
