package e2e

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The mock's former `background` parameter of spawn_agent is refused, not
// ignored (adr/fail-fast-unimplemented): spawn_agent always answers at once now.
func TestSpawnAgentRefusesTheOldBackgroundParameter(t *testing.T) {
	r := execMock(t, scenario{BypassTrust: true, Prompt: "go", Script: `#!/bin/sh
n=$(grep -c function_call_output "$A10N_MOCK_SESSION_FILE")
if [ "$n" = 0 ]; then
  printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"c0","name":"spawn_agent","input":{"message":"hi","background":true}}]}}'
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"done"}]}}' '{"type":"result","subtype":"success","result":"done"}'
`})
	assert.Contains(t, r.rollout(t), "'background' parameter is gone")
}
