package e2e

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Without --json, stdout carries only the final agent message (noninteractive,
// "make output machine-readable"): no events, no tool output, nothing a hook
// printed. (-o/--output-last-message is not modeled: the mock refuses it.)
// sr:proves noninteractive-run/codex
func TestExecWithoutJSONPrintsOnlyTheFinalMessage(t *testing.T) {
	r := execMock(t, scenario{
		NoJSON:    true,
		HooksJSON: hooksJSON("sh hook.sh", "SessionStart", "SessionEnd"),
		Files:     map[string]string{"hook.sh": `cat >/dev/null; echo HOOK-OUT`},
		Script:    callThenResult, Prompt: "go", Env: withCalls(t, "echo TOOL-OUT"),
	})
	require.Equal(t, 0, r.Code, r.Stderr)
	assert.Equal(t, "DONE\n", r.Stdout)
}

// What a SessionEnd hook prints on stdout, plain text or JSON additionalContext,
// is not recorded: it appears neither in the rollout nor as developer context,
// nor in the run's output.
// sr:proves session-end-hook/codex
func TestSessionEndHookOutputIsNotRecorded(t *testing.T) {
	for name, out := range map[string]string{
		"plain text":        `echo SE-PLAIN-OUT`,
		"additionalContext": `echo '{"hookSpecificOutput":{"hookEventName":"SessionEnd","additionalContext":"SE-JSON-OUT"}}'`,
	} {
		t.Run(name, func(t *testing.T) {
			r := execMock(t, scenario{
				HooksJSON: hooksJSON("sh hook.sh", "SessionEnd"),
				Files:     map[string]string{"hook.sh": `cat >>"$HOOK_LOG"; echo >>"$HOOK_LOG"; ` + out},
				Script:    callThenResult, Prompt: "go", Env: withCalls(t, "echo ran"),
			})
			require.Equal(t, 0, r.Code, r.Stderr)
			assert.Equal(t, "SessionEnd", r.hookLog()[len(r.hookLog())-1]["hook_event_name"], "the hook did not run")
			for _, secret := range []string{"SE-PLAIN-OUT", "SE-JSON-OUT"} {
				assert.NotContains(t, r.rollout(t), secret)
				assert.NotContains(t, r.Stdout, secret)
			}
			assert.Empty(t, developerTexts(t, r.rollout(t)))
		})
	}
}

// A SessionEnd hook is advisory: one that exits 2, or with any other failing
// status, neither blocks nor ends the run abnormally; the turn has already
// completed and the run exits 0 (hooks#sessionend).
// sr:proves hook-exit-code-semantics/codex
func TestSessionEndHookFailureDoesNotEndTheRunAbnormally(t *testing.T) {
	for _, code := range []string{"2", "1"} {
		r := execMock(t, scenario{
			HooksJSON: hooksJSON("sh hook.sh", "SessionEnd"),
			Files:     map[string]string{"hook.sh": `cat >>"$HOOK_LOG"; echo >>"$HOOK_LOG"; echo no >&2; exit ` + code},
			Script:    callThenResult, Prompt: "go", Env: withCalls(t, "echo ran"),
		})
		require.Equal(t, 0, r.Code, "exit %s: %s", code, r.Stderr)
		shape := streamShape(r.stream())
		assert.Equal(t, "turn.completed", shape[len(shape)-1], "exit %s", code)
		assert.Equal(t, "SessionEnd", r.hookLog()[len(r.hookLog())-1]["hook_event_name"])
	}
}

// A SessionEnd hook's matcher is applied to the reason the session ends, which for a
// non-interactive run is "other": a matcher of "other" runs, "clear" does not (hooks#sessionend).
// sr:proves hook-matcher-filter/codex
func TestSessionEndMatcherSelectsOnTheReason(t *testing.T) {
	for matcher, runs := range map[string]bool{"other": true, "^other$": true, "clear": false, "logout|clear": false} {
		t.Run(matcher, func(t *testing.T) {
			r := execMock(t, scenario{
				HooksJSON: `{"hooks":{"SessionEnd":[{"matcher":"` + matcher + `","hooks":[{"type":"command","command":"cat >>\"$HOOK_LOG\"; echo >>\"$HOOK_LOG\""}]}]}}`,
				Script:    callThenResult, Prompt: "go", Env: withCalls(t),
			})
			require.Equal(t, 0, r.Code, r.Stderr)
			ends := eventsOf(r, "SessionEnd")
			assert.Equal(t, runs, len(ends) == 1, "matcher %q", matcher)
			if runs {
				assert.Equal(t, "other", ends[0]["reason"])
			}
		})
	}
}
