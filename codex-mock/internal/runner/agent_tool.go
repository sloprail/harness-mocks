package runner

import (
	"github.com/sloprail/harness-mocks/internal/subagents"
	"github.com/sloprail/harness-mocks/internal/toolcall"
)

// agentTool is Codex's tool that dispatches a sub-agent. It needs a message
// (Codex also takes a list of items in its place; the mock does not model
// that): subagents.MessageRequired.
const agentTool = "spawn_agent"

// waitTool is the script's name for Codex's tool that waits for sub-agents to
// finish; the hooks name it waitHookName (recorded: runs/foreground-subagent-bash-ends-with-response).
const (
	waitTool     = "wait_agent"
	waitHookName = "multi_agent_v1wait_agent"
)

// waitRequired is what a wait must carry: the sub-agents to wait for.
var waitRequired = []string{"targets"}

// agentRequired is what a dispatch must carry, as the core checks it.
var agentRequired = subagents.MessageRequired

// spawnRefusal is what Codex tells the agent of a spawn_agent call that
// carries none of what it needs.
const spawnRefusal = "Provide one of: message or items"

// hookName is the tool's name in the hooks' payloads.
func hookName(c toolcall.Call) string {
	switch c.Name {
	case agentTool:
		return agentTool
	case waitTool:
		return waitHookName
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
