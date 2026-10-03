package e2e

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A session-start hook that says continue:false after a compaction does not
// stop the session, but ends the turn, cleanly: the compaction was made, the
// hook ran, and then no further command ran, no Stop hook fired and the model
// was not asked again; the stream completes the turn and the run exits 0 (not
// the abort a stopped PreCompact or PostCompact is). Recorded with the real
// harness under runs/session-start-compact-continue-false, whose hook printed
// continue:false only for source "compact", and replayed here.
// sr:proves session-start-hook/codex
func TestSessionStartAfterCompactionContinueFalseEndsTheTurn(t *testing.T) {
	rec, got := replayCompacting(t, "session-start-compact-continue-false")
	require.Equal(t, 0, got.Code, got.Stderr)

	want := hookShape(jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))))
	require.Equal(t, []string{"SessionStart:startup", "PreCompact:auto", "PostCompact:auto", "SessionStart:compact", "SessionEnd"}, want, "recorded")
	assert.Equal(t, want, hookShape(got.hookLog()), "the compact start was the last event before the session ended: no Stop hook")
	assert.Equal(t, "0\n", readFile(t, filepath.Join(rec.sample, "exit.txt")))

	assert.Equal(t, 1, compactedRecords(got.rollout(t)), "the compaction was made")
	assert.NotContains(t, got.rollout(t), `"reason":"interrupted"`, "not an aborted turn")
	assert.Equal(t, 1, strings.Count(got.Stdout, `"turn.completed"`))
	assert.Equal(t, 1, strings.Count(readFile(t, filepath.Join(rec.sample, "stream.jsonl")), `"turn.completed"`))
	cmds, _ := got.commands()
	assert.Len(t, cmds, 1, "no command after the one that led to the compaction (the recording ran one of three)")
}
