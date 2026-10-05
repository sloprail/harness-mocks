package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The recorded runs runs/tool-failure and runs/file-tools: commands exiting
// non-zero, a Read of a file that is not there, and file writes and reads that
// succeed.

// TestACommandExitingNonZeroFiresTheFailureHookInsteadOfTheSuccessHook:
// recorded, `false` and a command writing to stderr and exiting 3 each fire
// postToolUseFailure (failure_type error, not an interrupt) and no
// postToolUse; the message is the command's stderr, else "Command failed with
// exit code N".
// sr:proves tool-failure-hook/cursor
func TestACommandExitingNonZeroFiresTheFailureHookInsteadOfTheSuccessHook(t *testing.T) {
	got, want := replay(t, "tool-failure")
	conforms(t, got, want)

	for cmd, why := range map[string]string{"false": "Command failed with exit code 1", "sh -c 'echo OOPS >&2; exit 3'": "OOPS"} {
		require.Equal(t, "preToolUse beforeShellExecution afterShellExecution postToolUseFailure", joined(eventsOf(got, cmd)), cmd)
		msg, kind, _ := failureOf(got, cmd)
		require.Equal(t, why, msg, cmd)
		require.Equal(t, "error", kind, cmd)
	}
	for _, h := range got.hooks {
		if h["hook_event_name"] == "postToolUseFailure" {
			require.Equal(t, false, h["is_interrupt"])
		}
	}
	require.Equal(t, "tool_call/completed/shellToolCall/failure", got.frames[1])
}

// TestTheFailureHookSaysHowLongTheCallTook: recorded, postToolUseFailure
// carries duration, in milliseconds: the time a command that ran took, and 0
// for a call a hook refused before it ran.
// sr:proves tool-failure-hook/cursor
func TestTheFailureHookSaysHowLongTheCallTook(t *testing.T) {
	got, want := replay(t, "tool-failure")
	conforms(t, got, want)
	ran := 0
	for _, h := range got.raw {
		if h["hook_event_name"] != "postToolUseFailure" {
			continue
		}
		d, ok := h["duration"].(float64)
		require.True(t, ok, "duration in ms: %v", h)
		require.GreaterOrEqual(t, d, 0.0)
		ran++
	}
	require.Equal(t, 4, ran, "false, the stderr command, the missing read, and the write's read of a new file")

	refused, _ := replay(t, "pretool-refusal")
	for _, h := range refused.raw {
		if h["hook_event_name"] == "postToolUseFailure" {
			require.Equal(t, 0.0, h["duration"], "a refused call took no time")
		}
	}
}

// TestAFileToolsErrorFiresTheFailureHook: recorded, a Read of a missing file
// fires postToolUseFailure with "File not found: <path>", and a write first
// reads the file it changes, which is such a failure when the file is new.
// sr:proves tool-failure-hook/cursor
func TestAFileToolsErrorFiresTheFailureHook(t *testing.T) {
	got, want := replay(t, "tool-failure")
	conforms(t, got, want)

	var messages []string
	for _, h := range got.hooks {
		if h["hook_event_name"] == "postToolUseFailure" && h["tool_name"] == "Read" {
			messages = append(messages, h["error_message"].(string))
		}
	}
	require.Equal(t, []string{"File not found: <RUN>/missing-file.txt", "File not found: <RUN>/note.txt"}, messages)
}

// TestACallThatSucceedsFiresTheSuccessHook: recorded, a command exiting 0, a
// write and a read of an existing file fire postToolUse, not the failure hook,
// a write also fires afterFileEdit before it, and a read beforeReadFile.
// sr:proves tool-failure-hook/cursor
func TestACallThatSucceedsFiresTheSuccessHook(t *testing.T) {
	got, want := replay(t, "file-tools")
	conforms(t, got, want)

	require.Equal(t, "preToolUse afterFileEdit postToolUse", joined(fileToolEvents(got, "Write", 0)))
	require.Equal(t, "preToolUse beforeReadFile postToolUse", joined(fileToolEvents(got, "Read", 1)), "the second read: the file is there now, so beforeReadFile fires too")
	require.Equal(t, "tool_call/completed/readToolCall/success", got.frames[3])
}

// fileToolEvents are the events from the nth preToolUse of a tool to the next.
func fileToolEvents(o observed, tool string, nth int) (names []string) {
	seen, in := -1, false
	for _, h := range o.hooks {
		ev := h["hook_event_name"].(string)
		if ev == "preToolUse" {
			in = false
			if h["tool_name"] == tool {
				seen++
				in = seen == nth
			}
		}
		if in {
			names = append(names, ev)
		}
	}
	return names
}
