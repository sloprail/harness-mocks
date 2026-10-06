package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The recorded run runs/pretool-refusal-combined: four preToolUse hooks decide
// the same call, a deny and a block by exit status among them: for "echo BOTH"
// a deny (configured first) and an exit 2 (third), for "echo BLOCKFIRST" an
// exit 2 (first) and a deny, for "echo TWODENY" two denies; the hooks that had
// nothing to refuse decided nothing.

// TestEveryHookThatRefusesACallHasItsMessageToldInConfiguredOrder: recorded,
// when several preToolUse hooks refuse one call, a block by exit status does
// not outrank a deny in output or the other way round: the call is refused, and
// the failure hook's error_message holds the message of every hook that
// refused, in the order the hooks are configured in, one after another with a
// rule between them; the hooks that refused nothing add nothing.
// sr:proves pretooluse-refusal/cursor
func TestEveryHookThatRefusesACallHasItsMessageToldInConfiguredOrder(t *testing.T) {
	got, want := replay(t, "pretool-refusal-combined")
	conforms(t, got, want)

	for name, o := range map[string]observed{"recorded": want, "mock": got} {
		for cmd, why := range map[string]string{
			"echo BOTH":       "deny-one-DENY\n\n---\n\nHook blocked with message: block-two-BLOCK",
			"echo BLOCKFIRST": "Hook blocked with message: first-block-BLOCK\n\n---\n\ndeny-one-DENY",
			"echo TWODENY":    "deny-one-DENY\n\n---\n\ndeny-two-DENY",
		} {
			msg, kind, ok := failureOf(o, cmd)
			require.True(t, ok, name+": "+cmd)
			require.Equal(t, why, msg, name+": "+cmd)
			require.Equal(t, "permission_denied", kind, name+": "+cmd)
		}
		var ran []any
		for _, p := range postToolUses(o) {
			ran = append(ran, p["input"].(map[string]any)["command"])
		}
		require.Equal(t, []any{"echo FINE"}, ran, name+": only the call no hook refused ran")
		require.Equal(t, 3, countOf(o.frames, "tool_call/completed/shellToolCall/rejected"), name)
		require.Contains(t, o.frames, "tool_call/completed/shellToolCall/success", name)
	}
}

// TestSeveralHooksThatBlockOneCallAllRunAndTheirMessagesAreMerged: recorded
// (runs/pretool-refusal-combined), the four hooks all ran for each call, the
// blocking ones included, and the failure hook's error_message is neither the
// last-finishing blocker's nor the first's alone but the messages of both
// blockers joined in configuration order, a deny and an exit 2 both counting.
// sr:proves hooks-all-matching-run/cursor
func TestSeveralHooksThatBlockOneCallAllRunAndTheirMessagesAreMerged(t *testing.T) {
	got, want := replay(t, "pretool-refusal-combined")
	conforms(t, got, want)

	for name, o := range map[string]observed{"recorded": want, "mock": got} {
		for _, cmd := range []string{"echo BOTH", "echo BLOCKFIRST", "echo TWODENY"} {
			for _, hook := range []string{"first-block", "deny-one", "block-two", "deny-two"} {
				require.Contains(t, o.results, "ran:"+hook+":preToolUse:<nil>:"+cmd+":<nil>", name+": "+hook+" ran for "+cmd)
			}
		}
		msg, _, ok := failureOf(o, "echo BOTH")
		require.True(t, ok, name)
		require.Equal(t, "deny-one-DENY\n\n---\n\nHook blocked with message: block-two-BLOCK", msg, name)
	}
}

func countOf(names []string, name string) (n int) {
	for _, s := range names {
		if s == name {
			n++
		}
	}
	return n
}
