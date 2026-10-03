package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The recorded run runs/session-end-hook-output: a sessionEnd hook that prints
// a marker on stdout, a JSON object with a user_message, an agent_message and
// an additional_context, and a marker on stderr.

// TestWhatTheRecordedSessionEndHookPrintedIsNotInTheTranscript: recorded, the
// sessionEnd hook ran, once, last, saying "completed", and nothing of what it
// printed is anywhere in the sample: not in the transcript, not in the
// stream, not in the run's stderr. The mock's transcript has none of it
// either.
// sr:proves session-end-hook/cursor
func TestWhatTheRecordedSessionEndHookPrintedIsNotInTheTranscript(t *testing.T) {
	markers := []string{"SESSION-END-STDOUT-MARKER", "SESSION-END-JSON-MARKER", "SESSION-END-AGENT-MARKER", "SESSION-END-CONTEXT-MARKER", "SESSION-END-STDERR-MARKER"}
	got, want := replay(t, "session-end-hook-output")
	conforms(t, got, want)
	require.Len(t, want.hooks, 1)
	require.Equal(t, "sessionEnd", want.hooks[0]["hook_event_name"])
	require.Equal(t, "completed", want.hooks[0]["reason"])
	last := got.raw[len(got.raw)-1]
	require.Equal(t, "sessionEnd", last["hook_event_name"])
	require.Equal(t, "completed", last["reason"])
	require.Equal(t, "completed", last["final_status"])
	require.Equal(t, false, last["is_background_agent"])
	recorded := recordedRaw(t, "session-end-hook-output")
	end := recorded[len(recorded)-1]
	require.Equal(t, "sessionEnd", end["hook_event_name"])
	for name, p := range map[string]map[string]any{"recorded": end, "mock": last} {
		ms, ok := p["duration_ms"].(float64)
		require.True(t, ok, "%s sessionEnd carries duration_ms: %v", name, p)
		require.Greater(t, ms, float64(0), name)
	}

	samples, err := filepath.Glob(filepath.Join("..", "..", "snapshots", "runs", "session-end-hook-output", "samples", "*"))
	require.NoError(t, err)
	sample := samples[len(samples)-1]
	transcripts, err := filepath.Glob(filepath.Join(sample, "transcript", "*"))
	require.NoError(t, err)
	require.NotEmpty(t, transcripts, "the sample holds the transcript")
	scan := func(root string) {
		require.NoError(t, filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
			require.NoError(t, err)
			if info.IsDir() {
				return nil
			}
			b, err := os.ReadFile(p)
			require.NoError(t, err)
			for _, m := range markers {
				assert.NotContains(t, string(b), m, strings.TrimPrefix(p, root))
			}
			return nil
		}))
	}
	scan(sample)

	mockTranscripts, err := filepath.Glob(filepath.Join(got.home, ".cursor", "projects", "*", "agent-transcripts", "*", "*.jsonl"))
	require.NoError(t, err)
	require.NotEmpty(t, mockTranscripts, "the mock keeps a transcript")
	for _, p := range mockTranscripts {
		scan(p)
	}
}
