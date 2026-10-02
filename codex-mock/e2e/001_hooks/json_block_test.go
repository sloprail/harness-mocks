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
	assert.Contains(t, r.rollout(t), `"output":"FEEDBACK"`)
	assert.NotContains(t, r.rollout(t), `"output":"out\n"`)
}
