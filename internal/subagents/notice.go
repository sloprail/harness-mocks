package subagents

import "github.com/sloprail/harness-mocks/internal/tasks"

// Notice is a sub-agent that has ended without its owner having been told: its
// id and how it ended, its final answer or why it failed.
type Notice struct{ ID, Answer, Failure string }

// TakeNotices is what an agent is told, at the point after a tool output it was
// given, of the sub-agents it started that have ended since it was last told:
// each once, in the order they were started, whether or not the agent waited for
// it. Commands the agent left running are not sub-agents and are not told of here.
func TakeNotices(reg *tasks.Registry, owner string) (out []Notice) {
	for _, t := range reg.TakeFinished(owner) {
		if t.Kind == tasks.Agent {
			out = append(out, Notice{ID: t.ID, Answer: t.Result, Failure: t.Failure})
		}
	}
	return out
}
