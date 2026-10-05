package e2e

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestASessionStartHookFiresOnceAtTheStartAndCannotStopTheSession: recorded
// (runs/session-start-block: a sessionStart hook that says it blocks on stderr
// and exits 2, the status that blocks any other action), cursor-agent fires one
// sessionStart hook before anything else, the payload says no source (cursor
// does not tell a fresh session from a resumed one there) and has no transcript
// path yet, and the session goes on past the hook that tried to stop it: the
// command runs and its hooks fire, up to the sessionEnd one.
// sr:proves session-start-hook/cursor
func TestASessionStartHookFiresOnceAtTheStartAndCannotStopTheSession(t *testing.T) {
	got, want := replay(t, "session-start-block")
	conforms(t, got, want)

	const whole = "sessionStart preToolUse beforeShellExecution afterShellExecution postToolUse sessionEnd"
	require.Equal(t, whole, joined(hookNames(want)), "the recorded session went on after the start hook exited 2")
	require.Equal(t, whole, joined(hookNames(got)))
	require.Contains(t, want.results, "sessionStart:2")
	require.Contains(t, got.results, "sessionStart:2")

	recorded := readJSONL(t, filepath.Join(newestSample(t, "session-start-block"), "payloads.jsonl"))
	for name, start := range map[string]map[string]any{"recorded": recorded[0], "mock": got.raw[0]} {
		require.Equal(t, "sessionStart", start["hook_event_name"], name)
		require.Equal(t, false, start["is_background_agent"], name)
		require.NotContains(t, start, "source", name+": cursor's start payload does not say how the session began")
		require.Nil(t, start["transcript_path"], name+": no transcript path yet at the start")
	}
	require.Contains(t, recorded[0], "transcript_path", "recorded: the key is there, null")

	require.Contains(t, got.frames, "tool_call/completed/shellToolCall/success", "the command ran")
	require.Equal(t, "result/success", got.frames[len(got.frames)-1])
}

// TestTheSessionStartHookOfARunFromASymlinkedDirectoryFiresOnceWithNoTranscript:
// recorded (runs/symlinked-cwd: cursor-agent started from a symlink to the
// workspace), the sessionStart hook fires once, at the start, and its payload's
// transcript_path is null (the transcript file does not exist yet). The mock
// replays the run the same: one sessionStart, no transcript path.
// sr:proves session-start-hook/cursor
func TestTheSessionStartHookOfARunFromASymlinkedDirectoryFiresOnceWithNoTranscript(t *testing.T) {
	got, want := replay(t, "symlinked-cwd")
	conforms(t, got, want)

	recorded := readJSONL(t, filepath.Join(newestSample(t, "symlinked-cwd"), "payloads.jsonl"))
	for name, payloads := range map[string][]map[string]any{"recorded": recorded, "mock": got.raw} {
		var starts []map[string]any
		for _, h := range payloads {
			if h["hook_event_name"] == "sessionStart" {
				starts = append(starts, h)
			}
		}
		require.Len(t, starts, 1, name+": one sessionStart")
		require.Contains(t, starts[0], "transcript_path", name)
		require.Nil(t, starts[0]["transcript_path"], name+": no transcript yet at the start")
	}
}
