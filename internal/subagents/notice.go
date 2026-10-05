package subagents

import (
	"time"

	"github.com/sloprail/harness-mocks/internal/tasks"
)

// Notice is a sub-agent that has ended without its owner having been told: its
// id and how it ended, its final answer or why it failed.
type Notice struct{ ID, Answer, Failure string }

// TakeNotice is what an agent is told, at a point where the harness speaks to it (after
// a tool output it was given, or when its turn would end), of the sub-agents it
// started that have ended and it has not been told of: the first of them, in the
// order they were started, once, whether or not the agent waited for it. One
// at a time: the next is told at the next such point (recorded:
// runs/subagent-transcripts-v2, runs/foreground-subagent-wait-many). Commands the
// agent left running are not sub-agents and are not told of here.
func TakeNotice(reg *tasks.Registry, owner string) (Notice, bool) {
	for t := reg.TakeNext(owner); t != nil; t = reg.TakeNext(owner) {
		if t.Kind == tasks.Agent {
			return Notice{ID: t.ID, Answer: t.Result, Failure: t.Failure}, true
		}
	}
	return Notice{}, false
}

// AwaitNotice is TakeNotice at the point where the agent's turn would end: a
// model takes time to give its last answer, and a sub-agent that is still running
// may end within it, so it waits up to grace for one that does (while the agent has
// any running). A sub-agent that runs on past grace does not hold the turn up
// (recorded: runs/print-waits-for-background-agents, the turn ends with it running).
func AwaitNotice(reg *tasks.Registry, owner string, grace time.Duration) (Notice, bool) {
	deadline := time.Now().Add(grace)
	for {
		if n, ok := TakeNotice(reg, owner); ok {
			return n, true
		}
		if !reg.AgentsRunning(owner) || !time.Now().Before(deadline) {
			return Notice{}, false
		}
		time.Sleep(10 * time.Millisecond)
	}
}
