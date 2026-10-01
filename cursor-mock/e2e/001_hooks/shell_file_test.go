package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The recorded run runs/shell-exit-status: commands exiting 1 with no output,
// 1 with output, 2 with both streams, and 0.

// TestAShellCommandExitingNonZeroIsAFailureResultWithItsExitCodeAndOutput:
// recorded, whatever the status (a grep that finds nothing exits 1 and fails
// like any other), the tool's result is a failure, and the failure hook's
// message is what the command printed (stdout then stderr), or "Command failed
// with exit code N" when it printed nothing.
// sr:proves bash-tool-result/cursor
func TestAShellCommandExitingNonZeroIsAFailureResultWithItsExitCodeAndOutput(t *testing.T) {
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
	require.Len(t, edits, 2)
	require.Contains(t, got.frames, "tool_call/completed/readToolCall/success")
}

// TestAReadOfAFileThatIsNotThereIsAnErrorResult: recorded, the Read of a
// missing file fails with an error result, and the failure hook says so.
// sr:proves file-tools/cursor
func TestAReadOfAFileThatIsNotThereIsAnErrorResult(t *testing.T) {
	got, want := replay(t, "tool-failure")
	conforms(t, got, want)
	require.Contains(t, got.frames, "tool_call/completed/readToolCall/error")
}
