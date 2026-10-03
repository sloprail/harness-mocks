package runner

import (
	"context"

	"github.com/sloprail/harness-mocks/codex-mock/internal/hooks"
	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
	"github.com/sloprail/harness-mocks/internal/toolcall"
)

// agentTool is Codex's tool that dispatches a sub-agent.
const agentTool = "spawn_agent"

// spawnNeeds is what a spawn_agent call must carry: a message (Codex also
// takes a list of items in its place; the mock does not model that).
var spawnNeeds = []string{"message"}

// spawnRefusal is what Codex tells the agent of a spawn_agent call that
// carries none of what it needs.
const spawnRefusal = "Provide one of: message or items"

// beforeAgent fires PreToolUse for a sub-agent dispatch. Codex does not check
// the dispatch's input first: the hook sees the call as the agent made it,
// even one that carries nothing (recorded: runs/agent-input-validation).
func (h toolHost) beforeAgent(ctx context.Context, c toolcall.Call) (bool, string) {
	var ds []hooks.Decision
	own := map[string]any{"turn_id": h.turnID, "tool_name": agentTool, "tool_use_id": c.ID, "tool_input": c.Input}
	for _, o := range h.hooks.Fire(ctx, hooks.PreToolUse, agentTool, own) {
		ds = append(ds, hooks.Interpret(hooks.PreToolUse, o))
	}
	return hooks.Refusal(ds)
}

// executeAgent is the dispatch itself. A dispatch without its message is
// refused with the input-validation error, after the PreToolUse hook has run,
// and nothing runs (recorded: runs/agent-input-validation). The mock models no
// sub-agents, so a complete dispatch is answered as an error too.
// sr:provides agent-input-validation/codex
func executeAgent(c toolcall.Call) toolcall.Result {
	if len(corehooks.RejectedInput(c.Input, spawnNeeds)) > 0 {
		return toolcall.Result{Output: spawnRefusal, Failed: true}
	}
	return toolcall.Result{Output: "sub-agents are not modelled by this mock", Failed: true}
}
