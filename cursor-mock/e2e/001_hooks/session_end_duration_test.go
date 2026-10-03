package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestTheSessionEndHookGetsHowLongTheSessionLastedInMilliseconds: the doc
// lists duration_ms among the sessionEnd input fields, and the recorded
// sessionEnd payloads carry it (the replay's normalizer drops its value, which
// differs from run to run): the mock's hook gets it as a number, beside the
// reason and the final status.
// sr:proves session-end-hook/cursor
func TestTheSessionEndHookGetsHowLongTheSessionLastedInMilliseconds(t *testing.T) {
	got, _ := replay(t, "shell-exit-status")
	var seen int
	for _, h := range got.raw {
		if h["hook_event_name"] != "sessionEnd" {
			continue
		}
		seen++
		d, ok := h["duration_ms"].(float64)
		require.True(t, ok, "duration_ms is a number: %v", h)
		require.GreaterOrEqual(t, d, 0.0)
		require.Equal(t, "completed", h["reason"])
		require.Equal(t, "completed", h["final_status"])
	}
	require.Equal(t, 1, seen, "one sessionEnd")
}
