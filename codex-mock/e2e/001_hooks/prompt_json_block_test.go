package e2e

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAPromptRefusedByJSONRunsNothingAndSaysNothing: recorded in
// runs/user-prompt-submit-hook-json-block, a UserPromptSubmit hook that exits
// 0 printing {"decision":"block","reason":...} refuses the prompt as exit 2
// does: no command ran, the agent produced no message, neither the prompt nor
// the reason is in the rollout, and no Stop hook fired. The mock, given the
// command the prompt asks for, does the same.
// sr:proves user-prompt-submit-hook/codex
func TestAPromptRefusedByJSONRunsNothingAndSaysNothing(t *testing.T) {
	rec := loadRecording(t, "user-prompt-submit-hook-json-block")
	var fired []string
	for _, l := range jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))) {
		fired = append(fired, l["hook_event_name"].(string))
	}
	assert.Equal(t, []string{"UserPromptSubmit"}, fired)
	recStream := result{Stdout: readFile(t, filepath.Join(rec.sample, "stream.jsonl"))}
	recCmds, _ := recStream.commands()
	assert.Empty(t, recCmds)
	assert.NotContains(t, recStream.Stdout, "agent_message")
	files, err := filepath.Glob(filepath.Join(rec.sample, "transcript", "*"))
	require.NoError(t, err)
	require.NotEmpty(t, files)
	for _, p := range files {
		assert.NotContains(t, readFile(t, p), "SHOULDNOTRUN")
		assert.NotContains(t, readFile(t, p), "PROMPT-REFUSED-BY-JSON")
	}

	rec.calls = []string{"echo SHOULDNOTRUN"}
	got := replay(t, rec)
	require.Equal(t, 0, got.Code, got.Stderr)
	cmds, _ := got.commands()
	assert.Empty(t, cmds, "the refused prompt's command ran")
	assert.NotContains(t, got.Stdout, "agent_message")
	assert.NotContains(t, got.rollout(t), "SHOULDNOTRUN")
	assert.NotContains(t, got.rollout(t), "PROMPT-REFUSED-BY-JSON", "the reason does not reach the agent")
	var gotFired []string
	for _, l := range got.hookLog() {
		gotFired = append(gotFired, l["hook_event_name"].(string))
	}
	assert.Equal(t, fired, gotFired)
}

// TestWhatAPromptHookPrintsAsPlainTextReachesTheAgentBesideThePrompt: a
// UserPromptSubmit hook's plain-text output on exit 0 is context added to the
// prompt (recorded in runs/hook-exit-codes: the model answered with the hook's
// word), and the prompt itself is delivered unchanged: the mock's rollout
// holds the prompt as the user's message and the hook's text as context.
// sr:proves user-prompt-submit-hook/codex
func TestWhatAPromptHookPrintsAsPlainTextReachesTheAgentBesideThePrompt(t *testing.T) {
	r := execMock(t, scenario{
		HooksJSON: hooksJSON("sh hook.sh", "UserPromptSubmit"),
		Files:     map[string]string{"hook.sh": `cat >/dev/null; echo "CTX-FROM-PROMPT-HOOK"`},
		Script:    callThenResult, Prompt: "the original prompt", Env: withCalls(t),
	})
	require.Equal(t, 0, r.Code, r.Stderr)
	rollout := r.rollout(t)
	assert.Contains(t, rollout, "the original prompt")
	assert.Contains(t, rollout, "CTX-FROM-PROMPT-HOOK", "the hook's text reached the agent")

	j := execMock(t, scenario{
		HooksJSON: hooksJSON("sh hook.sh", "UserPromptSubmit"),
		Files:     map[string]string{"hook.sh": `cat >/dev/null; echo '{"hookSpecificOutput":{"hookEventName":"UserPromptSubmit","additionalContext":"CTX-FROM-JSON-HOOK"}}'`},
		Script:    callThenResult, Prompt: "the original prompt", Env: withCalls(t),
	})
	require.Equal(t, 0, j.Code, j.Stderr)
	assert.Contains(t, j.rollout(t), "the original prompt")
	assert.Contains(t, j.rollout(t), "CTX-FROM-JSON-HOOK", "additionalContext reached the agent (hooks#userpromptsubmit)")
	assert.NotContains(t, j.rollout(t), "hookSpecificOutput", "the JSON itself is not passed on")
}
