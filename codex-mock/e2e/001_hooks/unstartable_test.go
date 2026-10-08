package e2e

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// A hook that exits 1 (any status but 0 and 2) blocks nothing on any event:
// the prompt is submitted, the tool result stands, the stop is not refused
// (runs/hook-exit-codes: UserPromptSubmit, PostToolUse and Stop exiting 1).
// sr:proves hook-exit-code-semantics/codex
func TestExit1OnPromptToolResultAndStopBlocksNothing(t *testing.T) {
	r := execMock(t, scenario{
		HooksJSON: hooksJSON("sh hook.sh", "UserPromptSubmit", "PostToolUse", "Stop"),
		Files:     map[string]string{"hook.sh": `cat >/dev/null; echo "nope" >&2; exit 1`},
		Script:    callThenResult, Prompt: "go", Env: withCalls(t, "echo out"),
	})
	assert.Equal(t, 0, r.Code, r.Stderr)
	cmds, _ := r.commands()
	assert.Equal(t, []string{"echo out"}, cmds, "the prompt was blocked")
	assert.True(t, resultTold(t, r.rollout(t), "out\n", "nope"), "the tool result was replaced")
	assert.Equal(t, 1, strings.Count(r.Stdout, `"text":"DONE"`), "the stop was blocked and the turn continued")
}

// A hook whose command cannot be launched (command not found, exit 127) is a
// non-blocking failure: the tool call it guards still runs and the turn goes
// on (runs/hook-unstartable).
// sr:proves hook-exit-code-semantics/codex
func TestUnstartableHookDoesNotBlockTheCall(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "ran")
	r := execMock(t, scenario{
		HooksJSON: hooksJSON("/nonexistent/codex-mock-no-such-hook", "PreToolUse"),
		Script:    callThenResult, Prompt: "go", Env: withCalls(t, "touch "+marker),
	})
	assert.Equal(t, 0, r.Code, r.Stderr)
	assert.FileExists(t, marker, "an unstartable hook refused the call")
	assert.Contains(t, r.Stdout, `"text":"DONE"`, "the turn did not go on")
}

// A PreToolUse hook that exits 1 with stderr lines blocks nothing and its stderr is not shown: the
// call runs, the agent is told only the command's output, and the turn goes on (recorded:
// runs/hook-unstartable, whose hook prints two lines to stderr and exits 1).
// sr:proves hook-exit-code-semantics/codex
func TestPreToolUseExit1StderrBlocksNothingAndIsNotShown(t *testing.T) {
	rec := loadRecording(t, "hook-unstartable")
	for name, rollout := range map[string]string{"recorded": recordedRollout(t, rec)} {
		assert.NotContains(t, rollout, "line of stderr", name)
		assert.True(t, resultTold(t, rollout, "one\n", "stderr"), name+": the agent was told the output alone")
	}
	got := execMock(t, scenario{
		HooksJSON: readFile(t, filepath.Join(rec.setup, "hooks.json")),
		Files:     map[string]string{"hook.sh": readFile(t, filepath.Join(rec.setup, "hook.sh"))},
		Script:    callThenResult, Prompt: "go", Env: withCalls(t, "echo one"),
	})
	assert.Equal(t, 0, got.Code, got.Stderr)
	cmds, _ := got.commands()
	assert.Equal(t, []string{"echo one"}, cmds, "the call ran")
	assert.NotContains(t, got.rollout(t), "line of stderr")
	assert.NotContains(t, got.Stdout, "line of stderr")
	assert.True(t, resultTold(t, got.rollout(t), "one\n", "stderr"))
	assert.Contains(t, got.Stdout, `"text":"DONE"`)
}
