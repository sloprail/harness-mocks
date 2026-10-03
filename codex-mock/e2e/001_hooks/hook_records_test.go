package e2e

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The recorded runs runs/hook-exit-codes (every event's hook, with a context
// by plain text, a block by exit 2, errors by exit 1 and 2 and a stop that
// blocks once) and runs/hook-timeout (a hook killed by its timeout, a hook that
// succeeds silently).

// recordedRollout is the session file the real codex left for a recorded run.
func recordedRollout(t *testing.T, rec recording) string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(rec.sample, "transcript", "*.jsonl"))
	require.NoError(t, err)
	require.Len(t, files, 1)
	return readFile(t, files[0])
}

// addedContext is what hooks added to the conversation as developer messages:
// the real session starts with instruction messages of its own, which the
// mock has none of.
func addedContext(t *testing.T, rollout string) []string {
	t.Helper()
	var out []string
	for _, d := range developerTexts(t, rollout) {
		if !strings.HasPrefix(d, "<") {
			out = append(out, d)
		}
	}
	return out
}

// What a hook leaves in the session file is only what the agent is told: the
// context it adds, as a developer message; a block of a tool call, as that
// call's result; the reason a stop hook blocks with, as a hook_prompt user
// message. A hook that succeeds silently, one that fails without blocking
// (exit 1, a failure at SessionStart), and one stopped by its timeout leave
// nothing, and the end of a turn that ran hooks gets no summary record. The
// mock leaves exactly what the real codex did, for the same run
// (runs/hook-exit-codes, runs/hook-timeout).
// sr:proves hook-output-transcript-records/codex
func TestHookRunsLeaveOnlyWhatTheAgentWasTold(t *testing.T) {
	rec := loadRecording(t, "hook-exit-codes")
	want, got := recordedRollout(t, rec), replay(t, rec).rollout(t)

	for name, rollout := range map[string]string{"recorded": want, "mock": got} {
		assert.Equal(t, []string{"The secret word is BANANA."}, addedContext(t, rollout), name+": the context a hook added")
		assert.True(t, resultTold(t, rollout, "Command blocked by PreToolUse hook: Blocked: echo one is off in this scenario", "stderr"), name+": a block is the call's result")
		assert.Equal(t, 1, strings.Count(rollout, "<hook_prompt"), name+": the stop hook's reason")
		assert.Contains(t, rollout, "Before finishing, reply with the single word STOPPED-ONCE.", name)
		for _, quiet := range []string{"post-tool warning on exit 1", "session-start stderr on exit 2"} {
			assert.NotContains(t, rollout, quiet, name+": an error that blocked nothing was recorded")
		}
		assert.NotContains(t, strings.ToLower(rollout), "hook_summary", name+": a summary record")
	}

	timeout := loadRecording(t, "hook-timeout")
	wantT, gotT := recordedRollout(t, timeout), replay(t, timeout).rollout(t)
	for name, rollout := range map[string]string{"recorded": wantT, "mock": gotT} {
		assert.Empty(t, addedContext(t, rollout), name)
		for _, quiet := range []string{"TIMEOUT-DENY", "timed out", "grandchild", "hook_prompt"} {
			assert.NotContains(t, rollout, quiet, name+": a cancelled or silent hook left a record")
		}
		assert.True(t, resultTold(t, rollout, "one", "TIMEOUT"), name)
		assert.Len(t, toolOutputs(t, rollout), 1, name+": one result, for the one command")
	}
}
