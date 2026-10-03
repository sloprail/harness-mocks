package e2e

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEveryStopOfARecordedTurnCarriesTheTurnTheLastMessageAndTheContinuationFlag:
// recorded in runs/stops, a Stop hook that blocks twice sees three Stops in
// the one turn: the first with stop_hook_active false, the two that follow a
// block (by JSON, then by exit 2) with it true, each naming the same turn and
// carrying the agent's last message (what the agent said last: the recorded
// model's wording differs from the mock's script). The mock's three Stops
// carry the same fields and flags.
// sr:proves stop-hook-payload/codex
func TestEveryStopOfARecordedTurnCarriesTheTurnTheLastMessageAndTheContinuationFlag(t *testing.T) {
	rec := loadRecording(t, "stops")
	shape := func(stops []map[string]any) (active []any, turns map[any]bool) {
		turns = map[any]bool{}
		for _, s := range stops {
			active = append(active, s["stop_hook_active"])
			turns[s["turn_id"]] = true
			msg, ok := s["last_assistant_message"].(string)
			assert.True(t, ok && msg != "", "a Stop without a last message: %v", s)
		}
		return
	}
	wantActive, wantTurns := shape(recordedHooks(t, rec, "Stop"))
	assert.Equal(t, []any{false, true, true}, wantActive)
	assert.Len(t, wantTurns, 1)
	assert.NotContains(t, wantTurns, nil, "the recorded Stops name their turn")

	got := replay(t, rec)
	require.Equal(t, 0, got.Code, got.Stderr)
	gotActive, gotTurns := shape(eventsOf(got, "Stop"))
	assert.Equal(t, wantActive, gotActive)
	assert.Len(t, gotTurns, 1, "one turn")
	assert.NotContains(t, gotTurns, nil, "the mock's Stops name their turn")
}

// TestARefusedPromptRunsNothingAndSaysNothing: recorded in runs/prompt-blocked,
// a UserPromptSubmit hook that exits 2 on a prompt asking for a command: no
// command ran, the agent produced no message, the prompt is not in the
// rollout, and no Stop hook fired (the hooks that fired were SessionStart and
// UserPromptSubmit). The mock, given the command the prompt asks for, does
// the same.
// sr:proves user-prompt-submit-hook/codex
func TestARefusedPromptRunsNothingAndSaysNothing(t *testing.T) {
	rec := loadRecording(t, "prompt-blocked")
	var fired []string
	for _, l := range jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))) {
		fired = append(fired, l["hook_event_name"].(string))
	}
	assert.Equal(t, []string{"SessionStart", "UserPromptSubmit"}, fired)
	recStream := result{Stdout: readFile(t, filepath.Join(rec.sample, "stream.jsonl"))}
	recCmds, _ := recStream.commands()
	assert.Empty(t, recCmds)
	assert.NotContains(t, recStream.Stdout, "agent_message")
	for _, f := range []string{"transcript"} {
		files, err := filepath.Glob(filepath.Join(rec.sample, f, "*"))
		require.NoError(t, err)
		require.NotEmpty(t, files)
		for _, p := range files {
			assert.NotContains(t, readFile(t, p), "SHOULDNOTRUN", "the refused prompt is not in the rollout")
		}
	}

	rec.calls = []string{"echo SHOULDNOTRUN"} // the command the prompt asks for
	got := replay(t, rec)
	require.Equal(t, 0, got.Code, got.Stderr)
	cmds, _ := got.commands()
	assert.Empty(t, cmds, "the refused prompt's command ran")
	assert.NotContains(t, got.Stdout, "agent_message")
	assert.NotContains(t, got.rollout(t), "SHOULDNOTRUN")
	assert.NotContains(t, got.rollout(t), "PROMPT-BLOCK-MSG", "the exit-2 reason does not reach the agent")
	var gotFired []string
	for _, l := range got.hookLog() {
		gotFired = append(gotFired, l["hook_event_name"].(string))
	}
	assert.Equal(t, fired, gotFired)
}
