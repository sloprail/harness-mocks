package runner

import (
	"fmt"

	"github.com/sloprail/harness-mocks/claude-mock/internal/toolexec"
	"github.com/sloprail/harness-mocks/internal/subagents"
	"github.com/sloprail/harness-mocks/internal/tasks"
)

// concurrentLimitRefusal is the result of an Agent call made while the
// session's running sub-agents are at the limit (CLAUDE_CODE_MAX_CONCURRENT_SUBAGENTS,
// 20 by default): a tool error telling the agent not to retry, the call
// answered with PostToolUseFailure, and nothing spawned (recorded:
// snapshots/runs/bgagent-concurrent-limit). ok is false when it may spawn.
//
// sr:provides background-agent/claude
func concurrentLimitRefusal(cfg Config) (res toolexec.Result, ok bool) {
	limit := subagents.ConcurrentLimit(cfg.MaxConcurrentSubagents)
	running := 0
	if cfg.bg != nil {
		for _, t := range cfg.bg.Running() {
			if t.Kind == tasks.Agent {
				running++
			}
		}
	}
	if !subagents.AtConcurrentLimit(running, limit) {
		return res, false
	}
	msg := fmt.Sprintf("Concurrent subagent limit reached. You can run %d subagents at once. Do not retry. If the user wants more concurrent subagents, ask them to increase CLAUDE_CODE_MAX_CONCURRENT_SUBAGENTS.", limit)
	return toolexec.Result{Output: msg, IsError: true, Failed: true, ToolUseResult: "Error: " + msg}, true
}
