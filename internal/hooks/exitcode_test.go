package hooks

import "testing"

func TestVerdictOf(t *testing.T) {
	for code, want := range map[int]Verdict{0: Accepted, 2: Blocked, 1: NonBlockingError, 127: NonBlockingError, 3: NonBlockingError} {
		if got := VerdictOf(code); got != want {
			t.Errorf("VerdictOf(%d) = %d, want %d", code, got, want)
		}
	}
}
