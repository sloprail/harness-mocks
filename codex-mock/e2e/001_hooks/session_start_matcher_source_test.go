package e2e

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The start hook's matcher is applied to the source the session begins with, on a
// resumed and on a forked session too, not only on a fresh one and after a
// compaction (the docs: hooks#sessionstart; the recorded sources of a fresh, a resumed
// and a forked session: runs/session-resume, runs/session-fork, and the matcher
// on a fresh one: runs/hook-matchers). One hook per source marks that it ran.
const sourceMarker = `#!/bin/sh
cat >/dev/null
printf '{"ran":"%s"}\n' "$1" >>"$HOOK_LOG"
`

var sourceMatchers = `{"hooks":{"SessionStart":[
 {"matcher":"startup","hooks":[{"type":"command","command":"sh hook.sh startup"}]},
 {"matcher":"resume","hooks":[{"type":"command","command":"sh hook.sh resume"}]},
 {"matcher":"fork","hooks":[{"type":"command","command":"sh hook.sh fork"}]}]}}`

// sr:proves session-start-hook/codex
// sr:proves hook-matcher-filter/codex
func TestSessionStartMatcherIsAppliedToTheResumeAndForkSources(t *testing.T) {
	first := execMock(t, scenario{HooksJSON: sourceMatchers, Files: map[string]string{"hook.sh": sourceMarker},
		Script: callThenResult, Prompt: "go", Env: withCalls(t)})
	require.Equal(t, 0, first.Code, first.Stderr)
	assert.Equal(t, []string{"startup"}, marks(first.hookLog(), "ran"), "a fresh session: only the startup hook")
	id := threadIDs(first)[0]

	resumed := resumeIn(t, first, first.Repo, id, "again")
	require.Equal(t, 0, resumed.Code, resumed.Stderr)
	assert.Equal(t, []string{"resume", "startup"}, marks(first.hookLog(), "ran"), "a resumed session: only the resume hook runs, not startup's again")

	fork := execFork(t, first, id, "once more")
	require.NotEmpty(t, threadIDs(fork))
	assert.Equal(t, []string{"fork", "resume", "startup"}, marks(first.hookLog(), "ran"), "a forked session: only the fork hook runs")
}
