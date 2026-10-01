package hooks

import "testing"

func TestPreToolDecision(t *testing.T) {
	for _, tc := range []struct {
		name        string
		blocked     bool
		denied      bool
		wantRefused bool
		wantReason  string
	}{
		{"accepted", false, false, false, ""},
		{"blocked by status", true, false, true, "status"},
		{"denied in output", false, true, true, "output"},
		{"status outranks output", true, true, true, "status"},
	} {
		refused, reason := PreToolDecision(tc.blocked, "status", tc.denied, "output")
		if refused != tc.wantRefused || reason != tc.wantReason {
			t.Errorf("%s: got (%v, %q), want (%v, %q)", tc.name, refused, reason, tc.wantRefused, tc.wantReason)
		}
	}
}

func TestAfterToolHook(t *testing.T) {
	for o, want := range map[ToolOutcome]AfterTool{
		ToolRefused:   AfterNone,
		ToolSucceeded: AfterSuccess,
		ToolFailed:    AfterFailure,
		ToolErrored:   AfterNone,
	} {
		if got := AfterToolHook(o); got != want {
			t.Errorf("AfterToolHook(%d) = %d, want %d", o, got, want)
		}
	}
}

func TestStrongerPermission(t *testing.T) {
	for _, tc := range [][3]string{
		{"allow", "deny", "deny"}, {"deny", "allow", "deny"}, {"ask", "defer", "defer"},
		{"defer", "deny", "deny"}, {"", "allow", "allow"}, {"ask", "", "ask"},
	} {
		if got := StrongerPermission(tc[0], tc[1]); got != tc[2] {
			t.Errorf("StrongerPermission(%q, %q) = %q, want %q", tc[0], tc[1], got, tc[2])
		}
	}
}
