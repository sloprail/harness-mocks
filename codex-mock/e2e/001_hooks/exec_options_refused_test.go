package e2e

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An exec_command option the mock does not carry out is refused, not ignored
// (adr/fail-fast-unimplemented): a working directory other than the run's: the
// agent is told which, and the command does not run.
func TestExecCommandRefusesAnotherWorkingDirectory(t *testing.T) {
	r := execMock(t, scenario{BypassTrust: true, Prompt: "go", Script: `#!/bin/sh
n=$(grep -c function_call_output "$A10N_MOCK_SESSION_FILE")
if [ "$n" = 0 ]; then
  printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"c0","name":"Bash","input":{"command":"touch ran","workdir":"/"}}]}}'
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"done"}]}}' '{"type":"result","subtype":"success","result":"done"}'
`})
	assert.Contains(t, r.rollout(t), "exec_command workdir other than the run's directory is not implemented by the mock")
	assert.NoFileExists(t, r.Repo+"/ran")
}

// A command runs by the shell the call names: zsh sets ZSH_VERSION, and zsh -l is a login shell.
func TestExecCommandRunsByTheNamedShell(t *testing.T) {
	_, err := exec.LookPath("zsh")
	require.NoError(t, err, "zsh must be installed to run this suite (CI installs it): the mock refuses a call that names a shell it lacks")
	r := execMock(t, scenario{BypassTrust: true, Prompt: "go", Script: `#!/bin/sh
n=$(grep -c function_call_output "$A10N_MOCK_SESSION_FILE")
case "$n" in
0) printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"c0","name":"Bash","input":{"command":"echo V=${ZSH_VERSION:+zsh}; [[ -o login ]] && echo LOGIN || echo NOLOGIN","shell":"zsh","login":false}}]}}'; exit 0;;
1) printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"c1","name":"Bash","input":{"command":"[[ -o login ]] && echo LOGIN || echo NOLOGIN","shell":"zsh","login":true}}]}}'; exit 0;;
esac
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"done"}]}}' '{"type":"result","subtype":"success","result":"done"}'
`})
	cmds, _ := r.commands()
	require.Len(t, cmds, 2)
	var outs []string
	for _, e := range r.stream() {
		if item, _ := e["item"].(map[string]any); item["type"] == "command_execution" && e["type"] == "item.completed" {
			outs = append(outs, item["aggregated_output"].(string))
		}
	}
	require.Len(t, outs, 2)
	assert.Equal(t, "V=zsh\nNOLOGIN\n", outs[0])
	assert.Equal(t, "LOGIN\n", outs[1])
}

// A wait_agent call with a field the real tool has not, or one of the wrong type, fails
// with the reason; it is not read leniently.
func TestWaitAgentRefusesBadInput(t *testing.T) {
	r := execMock(t, scenario{BypassTrust: true, Prompt: "go", Script: `#!/bin/sh
n=$(grep -c function_call_output "$A10N_MOCK_SESSION_FILE")
if [ "$n" = 0 ]; then
  printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"c0","name":"wait_agent","input":{"targets":["x"],"timeout_ms":"soon","extra":1}}]}}'
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"done"}]}}' '{"type":"result","subtype":"success","result":"done"}'
`})
	assert.Contains(t, r.rollout(t), "wait_agent: invalid input")
}

// login:false with no shell named is accepted, as the real harness accepts it: the command runs by
// the default shell, not a login one (recorded: runs/exec-login-false, `zsh -c`). login:true with no
// shell is not recorded, so it stays refused.
func TestLoginFalseWithNoShellRunsTheDefaultShell(t *testing.T) {
	rec := loadRecording(t, "exec-login-false")
	got := replay(t, rec)
	require.Equal(t, 0, got.Code, got.Stderr)
	assert.NotContains(t, got.rollout(t), "is not implemented by the mock")
	cmds, _ := got.commands()
	assert.Len(t, cmds, 1)
	assert.Contains(t, readFile(t, filepath.Join(rec.sample, "stream.jsonl")), `"aggregated_output":"one\n"`)
	assert.Contains(t, got.Stdout, `"aggregated_output":"one\n"`)
}

// A PreToolUse hook refuses an exec_command (the shell tool, which hooks name Bash) however it was
// asked for, with the options the mock implements (a shell, a login flag, a yield time): the command
// does not run, by a deny or by exit 2 (hooks#tool-coverage, unified exec).
// sr:proves hook-matcher-filter/codex
// sr:proves pretooluse-refusal/codex
func TestAPreToolUseHookRefusesAnExecCommandWithItsOptions(t *testing.T) {
	for name, hook := range map[string]string{
		"deny by JSON": `cat >/dev/null; echo '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"no exec"}}'`,
		"exit 2":       `cat >/dev/null; echo "no exec" >&2; exit 2`,
	} {
		t.Run(name, func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "ran")
			r := execMock(t, scenario{
				HooksJSON: `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"sh hook.sh"}]}]}}`,
				Files:     map[string]string{"hook.sh": hook},
				Script: `#!/bin/sh
n=$(grep -c function_call_output "$A10N_MOCK_SESSION_FILE")
if [ "$n" = 0 ]; then
  printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"c0","name":"Bash","input":{"command":"touch ` + marker + `","shell":"zsh","login":false,"yield_time_ms":10000}}]}}'
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"done"}]}}' '{"type":"result","subtype":"success","result":"done"}'
`, Prompt: "go"})
			require.Equal(t, 0, r.Code, r.Stderr)
			assert.NoFileExists(t, marker, "the refused command ran")
			cmds, _ := r.commands()
			assert.Empty(t, cmds)
			assert.Contains(t, r.rollout(t), "Command blocked by PreToolUse hook: no exec.")
		})
	}
}
