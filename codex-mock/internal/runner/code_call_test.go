package runner

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sloprail/harness-mocks/codex-mock/internal/replay"
	"github.com/sloprail/harness-mocks/internal/scenario"
)

// calledAs is what the recordings' reader (the replay's) reads out of the JS the mock writes for a call
// of the script: the tool and its input in the recording's own names.
func calledAs(t *testing.T, name, input string) replay.RolloutCall {
	t.Helper()
	js := codeOf(scenario.ToolUse{ID: "c", Name: name, Input: json.RawMessage(input)})
	rec := map[string]any{"type": "response_item", "payload": map[string]any{"type": "custom_tool_call", "name": "exec", "input": js}}
	calls := replay.RolloutCalls([]map[string]any{rec})
	require.Len(t, calls, 1, js)
	return calls[0]
}

// A shell call is written as the recordings show it: exec_command with cmd, not Bash with command.
func TestAShellCallIsWrittenAsExecCommand(t *testing.T) {
	c := calledAs(t, "Bash", `{"command":"echo \"a\" && ls `+"`pwd`"+` <&>","workdir":"/w","yield_time_ms":10000}`)
	assert.Equal(t, "exec_command", c.Tool)
	assert.Equal(t, map[string]any{"cmd": "echo \"a\" && ls `pwd` <&>", "workdir": "/w", "yield_time_ms": float64(10000)}, c.Input)
}

// apply_patch takes the patch as its one argument.
func TestAPatchIsItsOneArgument(t *testing.T) {
	c := calledAs(t, "apply_patch", `{"command":"*** Begin Patch\n*** Add File: a\n+x\n*** End Patch"}`)
	assert.Equal(t, "apply_patch", c.Tool)
	assert.Equal(t, map[string]any{"arg": "*** Begin Patch\n*** Add File: a\n+x\n*** End Patch"}, c.Input)
}

// The sub-agent tools are written under their multi-agent names, and the mock's own script parameter is left out.
func TestAgentCallsAreWrittenUnderTheirRecordedNames(t *testing.T) {
	c := calledAs(t, "spawn_agent", `{"message":"hi","script":"sub1.sh"}`)
	assert.Equal(t, "multi_agent_v1__spawn_agent", c.Tool)
	assert.Equal(t, map[string]any{"message": "hi"}, c.Input)
	c = calledAs(t, "wait_agent", `{"targets":["a-1"],"timeout_ms":3600000}`)
	assert.Equal(t, "multi_agent_v1__wait_agent", c.Tool)
	assert.Equal(t, map[string]any{"targets": []any{"a-1"}, "timeout_ms": float64(3600000)}, c.Input)
	c = calledAs(t, "write_stdin", `{"session_id":89847,"yield_time_ms":10000}`)
	assert.Equal(t, "write_stdin", c.Tool)
	assert.Equal(t, map[string]any{"session_id": float64(89847), "yield_time_ms": float64(10000)}, c.Input)
}

// The call is the record Codex keeps of every one: a completed custom_tool_call of exec.
func TestTheJSIsTheShapeOfTheRecordings(t *testing.T) {
	js := codeOf(scenario.ToolUse{Name: "Bash", Input: json.RawMessage(`{"command":"true","workdir":"/w"}`)})
	assert.Equal(t, "const r = await tools.exec_command({cmd:\"true\",workdir:\"/w\"}); text(r.output);\n", js)
}
