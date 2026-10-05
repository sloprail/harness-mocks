package e2e

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
