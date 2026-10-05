package hooks

import "testing"

func ids() func() string {
	n := 0
	return func() string { n++; return string(rune('a' + n - 1)) }
}

// A prompt begins with a fresh id; the events that follow carry it; the
// permission mode is told only on the events about a turn.
func TestPromptFieldsFollowThePrompt(t *testing.T) {
	var turn Turn
	next := ids()
	if id, mode := PromptFields(&turn, next, PromptSurrounds, "m"); id != "" || mode != "" {
		t.Fatalf("before any prompt: %q %q", id, mode)
	}
	first, mode := PromptFields(&turn, next, PromptBegins, "m")
	if first != "a" || mode != "m" {
		t.Fatalf("begin: %q %q", first, mode)
	}
	if id, mode := PromptFields(&turn, next, PromptContinues, "m"); id != first || mode != "m" {
		t.Fatalf("continue: %q %q", id, mode)
	}
	if id, mode := PromptFields(&turn, next, PromptSurrounds, "m"); id != first || mode != "" {
		t.Fatalf("surround: %q %q", id, mode)
	}
	if id, _ := PromptFields(&turn, next, PromptBegins, "m"); id != "b" {
		t.Fatalf("a second prompt is a new id, got %q", id)
	}
}

// A compaction on a session with no prompt acts as one; with a prompt it keeps it.
func TestEnsureStartsAPromptOnlyWhenThereIsNone(t *testing.T) {
	var turn Turn
	next := ids()
	if got := turn.Ensure(next); got != "a" {
		t.Fatalf("none: %q", got)
	}
	if got := turn.Ensure(next); got != "a" {
		t.Fatalf("kept: %q", got)
	}
}
