package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A before-tool hook that refuses a call by JSON (or by exit 2): the command
// does not run, the event stream shows no command, the router logs the
// refusal, the agent is told it as the call's error, and the turn goes on to
// the script's next step and its end.
// sr:proves pretooluse-refusal/codex
func TestRefusedCallDoesNotRunAndTheTurnGoesOn(t *testing.T) {
	for name, hook := range map[string]string{
		"deny by JSON": `cat >/dev/null; echo '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"no thanks"}}'`,
		"exit 2":       `cat >/dev/null; echo "no thanks" >&2; exit 2`,
	} {
		t.Run(name, func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "ran")
			cmd := "touch " + marker
			r := execMock(t, scenario{
				HooksJSON: hooksJSON("sh hook.sh", "PreToolUse", "PostToolUse"),
				Files:     map[string]string{"hook.sh": hook},
				Script:    callThenResult, Prompt: "go",
				Env: withCalls(t, cmd),
			})
			require.Equal(t, 0, r.Code, r.Stderr)
			assert.NoFileExists(t, marker, "a refused command ran")
			cmds, _ := r.commands()
			assert.Empty(t, cmds, "the event stream shows a refused command")
			assert.Contains(t, r.Stderr, "Command blocked by PreToolUse hook: no thanks. Command: "+cmd)
			assert.Contains(t, r.rollout(t), "Command blocked by PreToolUse hook: no thanks. Command: "+cmd)
			assert.Contains(t, r.Stdout, `"text":"DONE"`, "the turn did not go on")
		})
	}
}

// Several hooks decide one call: a deny stands over an allow decided by
// another, and the reason is the denying hook's; a hook whose decision Codex
// does not support (ask) refuses nothing.
// sr:proves pretooluse-refusal/codex
func TestOneDenyAmongHooksRefusesAndAskDoesNot(t *testing.T) {
	rec := loadRecording(t, "pretool-decisions")
	r := replay(t, rec)
	cmds, _ := r.commands()
	assert.Equal(t, []string{"echo ONLYALLOW", "echo ASKME"}, cmds)
	assert.Contains(t, r.Stderr, "Command blocked by PreToolUse hook: deny by hook deny. Command: echo ALLOWDENY")
	// an allow or an ask leaves no record, in the recorded rollout or in the mock's
	for name, rollout := range map[string]string{"recorded": readFile(t, transcriptOf(t, rec)), "mock": r.rollout(t)} {
		assert.NotContains(t, rollout, "by hook ask", name)
		assert.NotContains(t, rollout, "by hook allow", name)
		assert.Contains(t, rollout, "deny by hook deny", name)
	}
}

// A hook that exits 1 with a deny printed does not refuse the call (recorded
// in runs/hook-exit-json: `echo b` ran).
// sr:proves hook-exit-code-semantics/codex
func TestExitOneWithDenyPrintedDoesNotRefuse(t *testing.T) {
	r := execMock(t, scenario{
		HooksJSON: hooksJSON("sh hook.sh", "PreToolUse"),
		Files:     map[string]string{"hook.sh": `cat >/dev/null; echo '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"x"}}'; exit 1`},
		Script:    callThenResult, Prompt: "go", Env: withCalls(t, "echo ran"),
	})
	cmds, _ := r.commands()
	assert.Equal(t, []string{"echo ran"}, cmds)
}

// A UserPromptSubmit hook exiting 2 blocks the prompt: no agent runs, the
// stream is a turn with nothing in it, and the run ends normally (recorded in
// runs/prompt-blocked). SessionStart exiting 2 blocks nothing, and Stop exiting
// 2 continues the turn with its stderr as the reason (runs/hook-exit-codes).
// sr:proves hook-exit-code-semantics/codex
func TestExit2PerEvent(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "script-ran")
	r := execMock(t, scenario{
		HooksJSON: hooksJSON("sh hook.sh", "UserPromptSubmit"),
		Files:     map[string]string{"hook.sh": `cat >/dev/null; echo blocked >&2; exit 2`},
		Script:    "touch " + marker + "\n" + callThenResult, Prompt: "go", Env: withCalls(t, "echo x"),
	})
	assert.Equal(t, 0, r.Code)
	assert.NoFileExists(t, marker, "the agent ran for a blocked prompt")
	assert.Equal(t, []string{"thread.started", "turn.started", "turn.completed"}, streamShape(r.stream()))

	r = execMock(t, scenario{
		HooksJSON: hooksJSON("sh hook.sh", "SessionStart", "Stop"),
		Files:     map[string]string{"hook.sh": `in=$(cat); case "$in" in *SessionStart*) echo "start says no" >&2; exit 2 ;; esac; if [ ! -f "$TMPDIR/once" ]; then : >"$TMPDIR/once"; echo "STOP-REASON" >&2; exit 2; fi`},
		Script:    callThenResult, Prompt: "go", Env: withCalls(t),
	})
	assert.Equal(t, 0, r.Code, r.Stderr)
	assert.Equal(t, 2, strings.Count(r.Stdout, `"text":"DONE"`), "the stop block did not continue the turn")
	// The recorded run (runs/stops) names the hook run `stop:<n>:<path of hooks.json>`;
	// the mock names it plain `stop`, so only the part both share is pinned: a
	// stop hook_prompt carrying the hook's reason.
	assert.Regexp(t, `<hook_prompt hook_run_id=\\"stop[^>]*>STOP-REASON</hook_prompt>`, r.rollout(t))
}

// PostToolUse fires for a command that failed too (runs/bashfail), and a hook
// that blocks it gives the agent its feedback in place of the result, the
// command having run (runs/posttool-block).
func TestPostToolUseFiresForFailureAndFeedbackReplacesTheResult(t *testing.T) {
	r := execMock(t, scenario{
		HooksJSON: hooksJSON("sh hook.sh", "PostToolUse"),
		Files:     map[string]string{"hook.sh": `cat >>"$HOOK_LOG"; echo >>"$HOOK_LOG"; echo "FEEDBACK" >&2; exit 2`},
		Script:    callThenResult, Prompt: "go", Env: withCalls(t, "echo out; exit 3"),
	})
	cmds, exits := r.commands()
	assert.Equal(t, []string{"echo out; exit 3"}, cmds)
	assert.Equal(t, []float64{3}, exits)
	assert.Len(t, r.hookLog(), 1, "PostToolUse did not fire for a failed command")
	assert.True(t, resultTold(t, r.rollout(t), "FEEDBACK", "out\n"), "the agent was not told the hook's feedback in place of the result")
}

// Commands run in the session's directory, hooks and the scenario script too.
func TestHooksRunInTheSessionDirectory(t *testing.T) {
	r := execMock(t, scenario{
		HooksJSON: hooksJSON(`pwd -P >"$TMPDIR/hook-dir"`, "SessionStart"),
		Script:    callThenResult, Prompt: "go", Env: withCalls(t),
	})
	got, err := os.ReadFile(filepath.Join(r.Tmp, "hook-dir"))
	require.NoError(t, err)
	assert.Equal(t, r.Repo, strings.TrimSpace(string(got)))
}
