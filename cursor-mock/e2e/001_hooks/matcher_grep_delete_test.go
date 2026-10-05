package e2e

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The recorded run runs/hook-matchers-grep-delete: a Grep call and a Delete
// call, with preToolUse hooks matched on Grep, Delete and Shell and one with no
// matcher, and postToolUse hooks matched on Grep and Delete.

// TestAMatcherIsTestedAgainstGrepAndDelete: recorded, a preToolUse or
// postToolUse hook whose matcher is Grep runs for the Grep call and one whose
// matcher is Delete for the Delete call, and the hook matched on Shell runs for
// neither; the hook with no matcher runs for both. The mock runs the same hooks
// for the same calls, and the Delete call removes the file.
// sr:proves hook-matcher-filter/cursor
func TestAMatcherIsTestedAgainstGrepAndDelete(t *testing.T) {
	got, want := replay(t, "hook-matchers-grep-delete")
	conforms(t, got, want)

	for name, o := range map[string]observed{"recorded": want, "mock": got} {
		for _, r := range []string{
			"ran:pre-Grep:preToolUse:Grep::<nil>", "ran:post-Grep:postToolUse:Grep::<nil>",
			"ran:pre-Delete:preToolUse:Delete::<nil>", "ran:post-Delete:postToolUse:Delete::<nil>",
		} {
			require.Contains(t, o.results, r, name)
		}
		var all int
		for _, r := range o.results {
			require.NotContains(t, r, "pre-Shell", name)
			if strings.HasPrefix(r, "ran:pre-all") {
				all++
			}
		}
		require.Equal(t, 2, all, name+": the hook with no matcher runs for both calls")
	}
	require.Equal(t, []string{"tool_call/started/grepToolCall/", "tool_call/completed/grepToolCall/success", "tool_call/started/deleteToolCall/", "tool_call/completed/deleteToolCall/success"}, completedAndStartedTools(got.frames))
}

// completedAndStartedTools are the frames of the Grep and Delete calls.
func completedAndStartedTools(frames []string) (out []string) {
	for _, f := range frames {
		if strings.Contains(f, "grepToolCall") || strings.Contains(f, "deleteToolCall") {
			out = append(out, f)
		}
	}
	return out
}
