package e2e

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEveryMatchingHookRunsAtTheSameTimeAsTheOthers: recorded (runs/hooks-together:
// two hooks of one event, each waiting for the other to start), each saw the
// other, which it can only have done if both were running at once.
// sr:proves hooks-all-matching-run/cursor
func TestEveryMatchingHookRunsAtTheSameTimeAsTheOthers(t *testing.T) {
	got, want := replay(t, "hooks-together")
	conforms(t, got, want)

	for name, o := range map[string]observed{"recorded": want, "mock": got} {
		require.Contains(t, o.results, "rv-a:beforeShellExecution:saw-b", name)
		require.Contains(t, o.results, "rv-b:beforeShellExecution:saw-a", name)
	}
}

// TestEveryMatchingHookRunsEvenWhenAnotherOneBlocks: recorded (runs/pretool-refusal:
// two preToolUse hooks, one of which blocks some calls by exit status or by a
// JSON deny), the other ran for every call, the blocked ones too.
// sr:proves hooks-all-matching-run/cursor
func TestEveryMatchingHookRunsEvenWhenAnotherOneBlocks(t *testing.T) {
	got, want := replay(t, "pretool-refusal")
	conforms(t, got, want)

	for name, o := range map[string]observed{"recorded": want, "mock": got} {
		var calls, others int
		for _, h := range o.hooks {
			if h["hook_event_name"] == "preToolUse" {
				calls++
			}
		}
		for _, r := range o.results {
			if strings.HasPrefix(r, "allow:preToolUse:") {
				others++
			}
		}
		require.NotZero(t, calls, name)
		require.Equal(t, calls, others, name+": the second hook ran for every call")
		require.Contains(t, o.results, "preToolUse:2", name+": and the first one blocked one")
	}
}
