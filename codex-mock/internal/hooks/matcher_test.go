package hooks

import (
	"context"
	"testing"
)

// fired is whether a hook of ev under the given matcher runs for match.
func fired(t *testing.T, ev Event, matcher, match string) bool {
	t.Helper()
	iv := &Invoker{Config: Config{ev: {{Matcher: matcher, Handlers: []Handler{{Command: "true"}}}}}, Dir: t.TempDir()}
	return len(iv.Fire(context.Background(), ev, match, nil)) == 1
}

// An apply_patch call also matches the matchers Edit and Write (hooks#matcher-patterns,
// Tool coverage table); a matcher that names neither does not select it.
// sr:proves hook-matcher-filter/codex
func TestApplyPatchAlsoMatchesEditAndWrite(t *testing.T) {
	for _, m := range []string{"apply_patch", "Edit", "Write", "Edit|Write", "^apply_patch$"} {
		if !fired(t, PreToolUse, m, "apply_patch") {
			t.Errorf("matcher %q did not select apply_patch", m)
		}
	}
	if fired(t, PreToolUse, "Bash", "apply_patch") || fired(t, PreToolUse, "Edit|Write", "Bash") {
		t.Error("a matcher selected a tool it does not name")
	}
}

// The matcher of SessionEnd is tested against its reason ("other" in a
// non-interactive run) and that of SessionStart against its source.
// sr:proves hook-matcher-filter/codex
func TestSessionMatchersSelectByReasonAndSource(t *testing.T) {
	if !fired(t, SessionEnd, "other", "other") || fired(t, SessionEnd, "clear", "other") {
		t.Error("SessionEnd matcher is not tested against the reason")
	}
	for _, src := range []string{"startup", "clear", "compact"} {
		if !fired(t, SessionStart, src, src) || fired(t, SessionStart, "startup|clear", "compact") {
			t.Errorf("SessionStart matcher for source %q", src)
		}
	}
}
