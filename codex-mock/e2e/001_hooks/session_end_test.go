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
