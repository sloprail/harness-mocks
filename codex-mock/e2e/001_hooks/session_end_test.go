package e2e

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
