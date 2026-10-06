package subagents

import (
	"context"
	"fmt"

	"github.com/sloprail/harness-mocks/internal/scenario"
	"github.com/sloprail/harness-mocks/internal/tasks"
)

// Hold holds an agent's step back until what its script's gate names has happened: the
// sub-agents it started (spawned) that must have ended, and how far the agent that
// started it (parent, nil for the session's own) must have got. A gate that names what
// does not exist is the script's mistake: it is returned as a problem to report, not
// waited for.
func Hold(ctx context.Context, g scenario.Gate, reg *tasks.Registry, spawned *SpawnLog, parent *Progress) (problems []string) {
	if g.ParentStarted > 0 || g.ParentDone > 0 || g.ParentEnded {
		if parent == nil {
			problems = append(problems, "a gate waits for the agent that started this one, and this one was started by none")
		} else {
			parent.Wait(ctx, g.ParentStarted, g.ParentDone)
			if g.ParentEnded {
				parent.WaitEnded(ctx)
			}
		}
	}
	for _, c := range g.ChildStarted {
		id, ok := spawned.at(c.Sub)
		if !ok {
			problems = append(problems, fmt.Sprintf("a gate waits for sub-agent %d, which this agent has not started", c.Sub))
			continue
		}
		if p := spawned.awaitProgress(ctx, id); p != nil {
			p.Wait(ctx, c.Calls, 0)
		}
	}
	for _, k := range g.Ended {
		id, ok := spawned.at(k)
		if !ok {
			problems = append(problems, fmt.Sprintf("a gate waits for sub-agent %d, which this agent has not started", k))
			continue
		}
		if !spawned.isSettled(id) {
			awaitEnd(ctx, reg, id)
		}
	}
	return problems
}

// awaitEnd waits for the sub-agent's task to end, or for ctx to.
func awaitEnd(ctx context.Context, reg *tasks.Registry, id string) {
	t := reg.Await(ctx, id) // its task is registered once its spawn has been answered
	if t == nil {
		return
	}
	select {
	case <-t.Done():
	case <-ctx.Done():
	}
}
