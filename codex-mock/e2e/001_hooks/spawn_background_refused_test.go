package e2e

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The mock's former `background` parameter of spawn_agent is refused, not
// ignored (adr/fail-fast-unimplemented, adr/tool-calls-validated): the schema has
// no such parameter, so the run fails naming it, before the call is played.
func TestSpawnAgentRefusesTheOldBackgroundParameter(t *testing.T) {
	r := execMock(t, scenario{BypassTrust: true, Prompt: "go", Files: map[string]string{"sub.sh": "#!/bin/sh\ntouch ran-anyway\n"}, Script: `#!/bin/sh
n=$(grep -c function_call_output "$A10N_MOCK_SESSION_FILE")
if [ "$n" = 0 ]; then
  printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"c0","name":"spawn_agent","input":{"message":"hi","background":true,"script":"sub.sh"}}]}}'
  exit 0
fi
if [ "$n" = 1 ]; then # keep the turn going, so a sub-agent started anyway would run
  printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"c1","name":"Bash","input":{"command":"sleep 1"}}]}}'
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"done"}]}}' '{"type":"result","subtype":"success","result":"done"}'
`})
	assert.NotEqual(t, 0, r.Code)
	assert.Contains(t, r.Stderr, `unknown parameter "background"`)
	assert.NoFileExists(t, filepath.Join(r.Repo, "ran-anyway"))
}
