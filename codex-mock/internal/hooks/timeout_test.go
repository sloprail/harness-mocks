package hooks

import (
	"context"
	"testing"
	"time"

	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
)

// fireTimeout is the limit a hook of ev with the given own timeout (seconds,
// zero for none) runs under, read off the outcome of a command that returns at once.
func fireTimeout(t *testing.T, ev Event, own int) time.Duration {
	t.Helper()
	iv := &Invoker{Config: Config{ev: {{Handlers: []Handler{{Command: "true", Timeout: own}}}}}, Dir: t.TempDir()}
	outs := iv.Fire(context.Background(), ev, "", nil)
	if len(outs) != 1 {
		t.Fatalf("outcomes: %d", len(outs))
	}
	return outs[0].Timeout
}

// A hook with no timeout of its own runs under Codex's default of 600 seconds
// (hooks#config-shape), whatever the event but SessionEnd; its own wins.
// sr:proves hook-timeout/codex
func TestDefaultTimeoutIs600Seconds(t *testing.T) {
	for _, ev := range []Event{SessionStart, UserPromptSubmit, PreToolUse, PostToolUse, Stop} {
		if got := fireTimeout(t, ev, 0); got != 600*time.Second {
			t.Errorf("%s default: %v", ev, got)
		}
	}
	if got := fireTimeout(t, PreToolUse, 7); got != 7*time.Second {
		t.Errorf("own timeout: %v", got)
	}
}

// firedOutcome runs one `sleep` command of ev with the given own timeout and
// returns its outcome.
func firedOutcome(t *testing.T, ev Event, own int) corehooks.Outcome {
	t.Helper()
	iv := &Invoker{Config: Config{ev: {{Handlers: []Handler{{Command: "sleep 30", Timeout: own}}}}}, Dir: t.TempDir()}
	outs := iv.Fire(context.Background(), ev, "", nil)
	if len(outs) != 1 {
		t.Fatalf("outcomes: %d", len(outs))
	}
	return outs[0]
}

// A SessionEnd hook that outlives its limit is killed at it: at 1 second when
// it has no timeout of its own, at 3 when its own is longer.
// sr:proves hook-timeout/codex
func TestSessionEndHookIsKilledAtItsLimit(t *testing.T) {
	for own, want := range map[int]time.Duration{0: time.Second, 10: 3 * time.Second} {
		o := firedOutcome(t, SessionEnd, own)
		if !o.TimedOut || o.Took < want || o.Took > want+2*time.Second {
			t.Errorf("own %ds: timed out %v after %v, want kill at %v", own, o.TimedOut, o.Took, want)
		}
	}
}

// A SessionEnd hook with no timeout of its own gets 1 second, and one with its
// own is held to 3 seconds at most (hooks#config-shape).
// sr:proves hook-timeout/codex
func TestSessionEndDefaultsToOneSecondCappedAtThree(t *testing.T) {
	cases := map[int]time.Duration{0: time.Second, 2: 2 * time.Second, 3: 3 * time.Second, 10: 3 * time.Second, 600: 3 * time.Second}
	for own, want := range cases {
		if got := fireTimeout(t, SessionEnd, own); got != want {
			t.Errorf("SessionEnd own %ds: %v, want %v", own, got, want)
		}
	}
}
