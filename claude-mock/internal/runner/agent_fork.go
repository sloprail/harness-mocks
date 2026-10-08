package runner

import (
	"encoding/json"
	"fmt"

	"github.com/sloprail/harness-mocks/claude-mock/internal/toolexec"
	"github.com/sloprail/harness-mocks/internal/subagents"
)

// A fork is the sub-agent type "fork": claude 2.1.285 with fork mode on
// (CLAUDE_CODE_FORK_SUBAGENT=1 in a `-p` session) runs it in the background
// whatever the call asked, writes its sidecar with agentType "fork", isFork
// true and model "inherit", and, unlike every other sub-agent, keeps its Agent
// tool at the depth limit, where a dispatch is refused with an error instead of
// "No such tool" (recorded: snapshots/runs/nested-fork-limit). The mock models
// that much of a fork, not the conversation it inherits.
const forkAgentType = "fork"

// isForkInput reports whether an Agent call asks for a fork.
func isForkInput(input json.RawMessage) bool {
	var in struct {
		SubagentType string `json:"subagent_type"`
	}
	return json.Unmarshal(input, &in) == nil && in.SubagentType == forkAgentType
}

// atSpawnLimit is what an Agent call gets when its dispatcher is at the depth
// limit: a fork is told the limit was reached, any other sub-agent has no Agent
// tool to call.
//
// sr:provides nested-subagents/claude
func atSpawnLimit(cfg Config) toolexec.Result {
	if cfg.AgentType != forkAgentType {
		return toolexec.Result{Output: "Error: No such tool available: Agent", IsError: true}
	}
	limit := cfg.SpawnLimit
	if limit == 0 {
		limit = subagents.DefaultSpawnLimit
	}
	if cfg.bg != nil {
		cfg.bg.stats.RefuseDepth()
	}
	// The call ran and failed: PostToolUseFailure fires for it (recorded).
	msg := fmt.Sprintf(
		"Subagent nesting limit reached (depth %d of %d). Complete this task directly using your tools instead of spawning another agent. If the user explicitly requested deeper nesting, ask them to raise CLAUDE_CODE_MAX_SUBAGENT_SPAWN_DEPTH.",
		cfg.spawnDepth, limit)
	return toolexec.Result{IsError: true, Failed: true, Output: msg, ToolUseResult: "Error: " + msg}
}
