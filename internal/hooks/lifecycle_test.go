package hooks

import "testing"

func TestSessionStartKind(t *testing.T) {
	for _, tc := range []struct {
		resumed, forked, compacted bool
		want                       StartKind
	}{
		{false, false, false, StartFresh},
		{true, false, false, StartResumed},
		{true, true, false, StartForked},
		{false, true, false, StartForked},
		{false, false, true, StartCompacted},
		{true, false, true, StartCompacted},
	} {
		if got := SessionStartKind(tc.resumed, tc.forked, tc.compacted); got != tc.want {
			t.Errorf("SessionStartKind(%v,%v,%v) = %d, want %d", tc.resumed, tc.forked, tc.compacted, got, tc.want)
		}
	}
}

func TestBlocksSessionStart(t *testing.T) {
	for _, v := range []Verdict{Accepted, Blocked, NonBlockingError} {
		if BlocksSessionStart(v) {
			t.Errorf("a %d verdict stopped the session from starting", v)
		}
	}
}

func TestStartHookEndsTurn(t *testing.T) {
	for _, tc := range []struct{ said, honoured, want bool }{
		{true, true, true}, {true, false, false}, {false, true, false}, {false, false, false},
	} {
		if got := StartHookEndsTurn(tc.said, tc.honoured); got != tc.want {
			t.Errorf("StartHookEndsTurn(%v, %v) = %v, want %v", tc.said, tc.honoured, got, tc.want)
		}
	}
}

func TestSessionEndReason(t *testing.T) {
	if got := SessionEndReason(false, EndInteractive); got != EndOther {
		t.Errorf("non-interactive: %d", got)
	}
	if got := SessionEndReason(true, EndInteractive); got != EndInteractive {
		t.Errorf("interactive: %d", got)
	}
	if SessionEndRecordsOutput() {
		t.Error("a session-end hook's output must not be recorded")
	}
}

func TestPrompt(t *testing.T) {
	for src, want := range map[PromptSource]bool{PromptFromUser: true, PromptTaskNotification: true, PromptSubagentDispatch: false} {
		if got := PromptHookFires(src); got != want {
			t.Errorf("PromptHookFires(%d) = %v, want %v", src, got, want)
		}
	}
	if refused, extra := PromptOutcome(true, "ctx"); !refused || extra != "" {
		t.Errorf("blocked: %v %q", refused, extra)
	}
	if refused, extra := PromptOutcome(false, "ctx"); refused || extra != "ctx" {
		t.Errorf("accepted: %v %q", refused, extra)
	}
}

func TestContext(t *testing.T) {
	for _, tc := range []struct {
		structured, plain string
		plainAdds         bool
		want              string
	}{
		{"json", "plain", true, "json\nplain"},
		{"", "plain", true, "plain"},
		{"", "plain", false, ""},
		{"json", "plain", false, "json"},
	} {
		if got := ContextOf(tc.structured, tc.plain, tc.plainAdds); got != tc.want {
			t.Errorf("ContextOf(%q,%q,%v) = %q, want %q", tc.structured, tc.plain, tc.plainAdds, got, tc.want)
		}
	}
	for _, tc := range [][3]string{{"", "", ""}, {"a", "", "a"}, {"", "b", "b"}, {"a", "b", "a\nb"}} {
		if got := JoinContext(tc[0], tc[1]); got != tc[2] {
			t.Errorf("JoinContext(%q,%q) = %q, want %q", tc[0], tc[1], got, tc[2])
		}
	}
}
