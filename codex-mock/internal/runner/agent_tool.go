package runner

import (
	"github.com/sloprail/harness-mocks/internal/subagents"
	"github.com/sloprail/harness-mocks/internal/toolcall"
)

// agentTool is Codex's tool that dispatches a sub-agent. It needs a message
// (Codex also takes a list of items in its place; the mock does not model
// that): subagents.MessageRequired.
const agentTool = "spawn_agent"

// agentRequired is what a dispatch must carry, as the core checks it.
var agentRequired = subagents.MessageRequired

// spawnRefusal is what Codex tells the agent of a spawn_agent call that
// carries none of what it needs.
const spawnRefusal = "Provide one of: message or items"

// hookName is the tool's name in the hooks' payloads.
func hookName(c toolcall.Call) string {
	if c.Name == agentTool {
		return agentTool
	}
	return toolName
}

// InputCheckedLate: Codex checks a dispatch's input only after the PreToolUse
// hook has seen the call, even one that carries nothing; the call is then
// refused with spawnRefusal, nothing runs and no PostToolUse hook fires
// (recorded: runs/agent-input-validation). The shell tool's input is checked
// first.
// sr:provides agent-input-validation/codex
func (h toolHost) InputCheckedLate(name string) bool { return name == agentTool }
