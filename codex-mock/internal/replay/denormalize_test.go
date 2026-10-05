package replay

import (
	"testing"

	"github.com/stretchr/testify/assert"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

func answer(s string) core.Call {
	return core.Call{Tool: core.ToolAnswer, Input: map[string]any{"text": s}}
}

// A run's answers are steps of the script like its calls: a turn a Stop hook
// continues is followed by the model's next steps (recorded: runs/stops).
func TestAnswersAreSteps(t *testing.T) {
	rec := core.Recording{Agent: core.Agent{Calls: []core.Call{
		{Tool: core.ToolShell, Input: map[string]any{"command": "echo a"}}, answer("DONE"),
	}, Final: "DONE2"}}
	script := Denormalize(rec).Script
	assert.Contains(t, script, `{"final":"DONE","gate":{}}`)
	assert.Contains(t, script, `{"final":"DONE2","gate":{}}`)
	assert.Contains(t, script, `<hook_prompt`)
}

// A recording that ends on a call still ends the script with an answer, so the
// last call is not played again.
func TestScriptEndsWithAnAnswer(t *testing.T) {
	rec := core.Recording{Agent: core.Agent{Calls: []core.Call{{Tool: core.ToolShell, Input: map[string]any{"command": "echo a"}}}}}
	assert.Contains(t, Denormalize(rec).Script, `{"final":"","gate":{}}`)
}
