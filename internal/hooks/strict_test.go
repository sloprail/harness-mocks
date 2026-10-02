package hooks

import "testing"

func TestFailsClosedOnlyForATimedOutCommandUnderTheStrictSetting(t *testing.T) {
	timedOut := Outcome{Started: true, TimedOut: true}
	done := Outcome{Started: true}
	if FailsClosed(timedOut, false) || !FailsClosed(timedOut, true) || FailsClosed(done, true) {
		t.Error("only a timed-out command under the strict setting fails closed")
	}
}

func TestSilentFailsOnlyForNoOutputUnderTheStrictSetting(t *testing.T) {
	if SilentFails("  \n", false) || !SilentFails("  \n", true) || SilentFails("{}", true) {
		t.Error("only no output under the strict setting fails")
	}
}
