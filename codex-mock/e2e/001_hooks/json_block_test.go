package e2e

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The docs (hooks#userpromptsubmit) say a UserPromptSubmit hook that prints
// {"decision":"block"} blocks the prompt as exit 2 does; the recorded runs
// cover only the exit-2 path, so this one is the mock's reading of the docs.
// sr:proves hook-exit-code-semantics/codex
func TestUserPromptSubmitJSONBlockBlocksThePrompt(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "script-ran")
	r := execMock(t, scenario{
		HooksJSON: hooksJSON("sh hook.sh", "UserPromptSubmit"),
		Files:     map[string]string{"hook.sh": `cat >/dev/null; echo '{"decision":"block","reason":"no thanks"}'`},
		Script:    "touch " + marker + "\n" + callThenResult, Prompt: "go", Env: withCalls(t, "echo x"),
	})
	assert.Equal(t, 0, r.Code, r.Stderr)
	assert.NoFileExists(t, marker, "the agent ran for a blocked prompt")
	assert.Equal(t, []string{"thread.started", "turn.started", "turn.completed"}, streamShape(r.stream()))
}

// The docs (hooks#posttooluse) say a PostToolUse hook printing
// {"decision":"block"} replaces the tool result with its reason, the command
// having run, as exit 2 does (the recorded run covers exit 2 only).
// sr:proves hook-exit-code-semantics/codex
func TestPostToolUseJSONBlockReplacesTheResult(t *testing.T) {
	r := execMock(t, scenario{
		HooksJSON: hooksJSON("sh hook.sh", "PostToolUse"),
		Files:     map[string]string{"hook.sh": `cat >/dev/null; echo '{"decision":"block","reason":"FEEDBACK"}'`},
		Script:    callThenResult, Prompt: "go", Env: withCalls(t, "echo out"),
	})
	assert.Equal(t, 0, r.Code, r.Stderr)
	cmds, _ := r.commands()
	assert.Equal(t, []string{"echo out"}, cmds)
	assert.True(t, resultTold(t, r.rollout(t), "FEEDBACK", "out\n"), "the agent was not told the hook's feedback in place of the result")
}

// The legacy {"decision":"block"} shape on PreToolUse refuses the call as a
// deny does: the command does not run and the agent is told the reason.
// sr:proves pretooluse-refusal/codex
func TestPreToolUseLegacyBlockRefusesTheCall(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "ran")
	cmd := "touch " + marker
	r := execMock(t, scenario{
		HooksJSON: hooksJSON("sh hook.sh", "PreToolUse"),
		Files:     map[string]string{"hook.sh": `cat >/dev/null; echo '{"decision":"block","reason":"legacy no"}'`},
		Script:    callThenResult, Prompt: "go", Env: withCalls(t, cmd),
	})
	assert.Equal(t, 0, r.Code, r.Stderr)
	assert.NoFileExists(t, marker, "a refused command ran")
	assert.Contains(t, r.rollout(t), "Command blocked by PreToolUse hook: legacy no. Command: "+cmd)
}

// A PreToolUse hook returning continue:false, stopReason or suppressOutput is
// marked failed and the tool call continues: nothing is refused.
// sr:proves pretooluse-refusal/codex
func TestPreToolUseContinueFalseDoesNotRefuseTheCall(t *testing.T) {
	for name, out := range map[string]string{
		"continue false": `{"continue":false,"stopReason":"halt"}`,
		"stopReason":     `{"stopReason":"halt"}`,
		"suppressOutput": `{"suppressOutput":true}`,
		"legacy approve": `{"decision":"approve","reason":"ok"}`,
	} {
		t.Run(name, func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "ran")
			r := execMock(t, scenario{
				HooksJSON: hooksJSON("sh hook.sh", "PreToolUse"),
				Files:     map[string]string{"hook.sh": `cat >/dev/null; echo '` + out + `'`},
				Script:    callThenResult, Prompt: "go", Env: withCalls(t, "touch "+marker),
			})
			assert.Equal(t, 0, r.Code, r.Stderr)
			assert.FileExists(t, marker, "the call was refused")
			assert.NotContains(t, r.rollout(t), "Command blocked by PreToolUse hook")
		})
	}
}
