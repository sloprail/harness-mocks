package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestARecordedNonInteractiveRunReplaysToTheSameToolCalls: recorded
// (runs/pretool-refusal), a single prompt run to completion without
// interaction makes the tool calls the agent made, in order; replayed, the
// mock's stream shows the same calls, and what its hooks saw conforms to the
// recording.
// sr:proves noninteractive-run/cursor
func TestARecordedNonInteractiveRunReplaysToTheSameToolCalls(t *testing.T) {
	got, want := replay(t, "pretool-refusal")
	conforms(t, got, want)

	require.NotEmpty(t, want.frames)
	require.Equal(t, want.frames, got.frames)
}

// TestThePrintFlagInItsLongFormRunsNonInteractively: recorded
// (runs/print-long-form), cursor-agent takes print mode as -p or --print (the
// headless doc's "-p, --print"); a run given both ends with the command made
// and its hooks fired as with -p alone. The mock does the same.
// sr:proves noninteractive-run/cursor
func TestThePrintFlagInItsLongFormRunsNonInteractively(t *testing.T) {
	got, want := replayWith(t, "print-long-form", "--print")
	conforms(t, got, want)

	require.NotEmpty(t, want.frames)
	require.Equal(t, want.frames, got.frames)
}
