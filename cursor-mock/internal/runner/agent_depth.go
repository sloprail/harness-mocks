package runner

import (
	"context"

	"github.com/sloprail/harness-mocks/cursor-mock/internal/toolexec"
	"github.com/sloprail/harness-mocks/internal/scenario"
	"github.com/sloprail/harness-mocks/internal/subagents"
)

// agentDepthLimit is how many layers of sub-agents Cursor lets a session nest:
// two. The main agent's sub-agent can start one of its own, and that one is not
// offered the tool (recorded: runs/nested-subagents-depth, where it said no Task
// tool is available).
const agentDepthLimit = 2

// depth is how many layers of sub-agents deep the session is: 0 for the main
// agent, one more for each parent a sub-agent has.
func (s *session) depth() int {
	d := 0
	for p := s.parent; p != nil; p = p.parent {
		d++
	}
	return d
}

// refusesTaskAtTheLimit answers a Task call of a sub-agent that is at the depth
// limit as a call of a tool it does not have, which it is not offered: it is on
// the stream and the transcript and no hook fires. It reports whether it did.
// The dispatch and its placement one layer deeper are the core's
// (subagents.Place; the sub-agent's parent chain is its depth).
// sr:provides nested-subagents/cursor
func (s *session) refusesTaskAtTheLimit(_ context.Context, tu scenario.ToolUse) bool {
	c := toolexec.FromScript(tu.Name, tu.Input)
	if c.Kind != "taskToolCall" || (subagents.Parent{ID: s.id, Depth: s.depth()}).CanDispatch(agentDepthLimit) {
		return false
	}
	s.forward(startedFrame(s.id, tu.ID, c))
	s.tr.toolUse(tu.Name, c.Args)
	s.forward(errorFrame(s.id, tu.ID, toolexec.Call{Kind: "unknownToolCall"}, "Unknown tool: "+tu.Name, nil))
	return true
}
