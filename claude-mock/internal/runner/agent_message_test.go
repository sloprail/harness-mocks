package runner

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The harness fills type, recipient and content into a message; its PreToolUse hook sees a summary
// too, its PostToolUse hook only to, message and summary (recorded: runs/fgsub-maxturns).
func TestAMessageIsFilledInAndHookedAsRecorded(t *testing.T) {
	in := map[string]any{"to": "a1", "message": "go"}
	assert.True(t, messageDefaults(in))
	assert.Equal(t, map[string]any{"to": "a1", "message": "go", "type": "message", "recipient": "a1", "content": "go"}, in)

	raw := json.RawMessage(`{"to":"a1","message":"go","type":"message","recipient":"a1","content":"go"}`)
	var pre, post map[string]any
	require.NoError(t, json.Unmarshal(hookInput(true, "SendMessage", raw), &pre))
	require.NoError(t, json.Unmarshal(hookInput(false, "SendMessage", raw), &post))
	assert.Equal(t, "go", pre["summary"])
	assert.Equal(t, map[string]any{"to": "a1", "message": "go", "summary": "go"}, post)
	assert.JSONEq(t, `{"command":"x"}`, string(hookInput(true, "Bash", json.RawMessage(`{"command":"x"}`))))
}

// A script names the agent a message goes to by its position among the sub-agents the agent started: the
// run mints the ids.
func TestAMessageNamesItsAgentByPosition(t *testing.T) {
	cfg := Config{steps: newAgentSteps(nil)}
	cfg.steps.spawn("aaa", true)
	cfg.steps.spawn("bbb", false)
	in := map[string]any{"to": "spawn:1", "recipient": "spawn:1", "message": "go"}
	blocks := []any{map[string]any{"type": "tool_use", "name": "SendMessage", "input": in}}
	assert.True(t, nameSpawned(cfg, blocks))
	assert.Equal(t, "bbb", in["to"])
	assert.Equal(t, "bbb", in["recipient"])
	assert.False(t, nameSpawned(cfg, blocks), "an id is left as it is")
}
