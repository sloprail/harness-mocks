package e2e

import (
	"os/exec"
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
