package replay

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnifyWaitNamesTheSpawnsByPosition(t *testing.T) {
	spawns, told := []int{0, 3}, []string{"id-a", "id-b"}
	c, err := unifyWait(map[string]any{"targets": []any{ref{call: 3, path: ".agent_id"}, "id-a"}, "timeout_ms": number{5000}}, spawns, told)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"targets": []int{1, 0}, "timeout_ms": 5000}, c.Input)
}

func TestUnifyWaitRefusesWhatItCannotName(t *testing.T) {
	for name, arg := range map[string]map[string]any{
		"an id nobody was told of":       {"targets": []any{"nope"}, "timeout_ms": number{1}},
		"the receipt of another call":    {"targets": []any{ref{call: 1, path: ".agent_id"}}, "timeout_ms": number{1}},
		"another part of the receipt":    {"targets": []any{ref{call: 0, path: ".nickname"}}, "timeout_ms": number{1}},
		"targets that are not a list":    {"targets": "x", "timeout_ms": number{1}},
		"a timeout that is not a number": {"targets": []any{}, "timeout_ms": "x"},
	} {
		_, err := unifyWait(arg, []int{0}, []string{"id-a"})
		assert.Error(t, err, name)
	}
}

func TestAgentIDsAreTheReceiptsTheScriptPrinted(t *testing.T) {
	out := []any{
		map[string]any{"type": "input_text", "text": "Script completed\nOutput:\n"},
		map[string]any{"type": "input_text", "text": `{"agent_id":"x1","nickname":"James"}`},
	}
	assert.Equal(t, []string{"x1"}, agentIDs(out))
	assert.Empty(t, agentIDs("not a list"))
}
