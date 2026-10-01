package hooks

import "testing"

func TestVerdictOf(t *testing.T) {
	for code, want := range map[int]Verdict{0: Accepted, 2: Blocked, 1: NonBlockingError, 127: NonBlockingError, 3: NonBlockingError} {
		if got := VerdictOf(code); got != want {
			t.Errorf("VerdictOf(%d) = %d, want %d", code, got, want)
		}
	}
}

func TestIsJSONOutput(t *testing.T) {
	for in, want := range map[string]bool{`{"a":1}`: true, "  {}\n": true, "The secret word is BANANA.": false, `{"a":1`: false, `["x"]`: false, `"q"`: false, "": false} {
		if got := IsJSONOutput(in); got != want {
			t.Errorf("IsJSONOutput(%q) = %v, want %v", in, got, want)
		}
	}
}
