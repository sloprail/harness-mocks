package subagents

import (
	"context"
	"fmt"
	"sync"

	"github.com/sloprail/harness-mocks/internal/scenario"
	"github.com/sloprail/harness-mocks/internal/tasks"
)

// Progress is how far an agent has got through its tool calls: how many it has
// started and how many it has finished. What another agent's script says it must
// wait for (scenario.Gate) is read against it, so the order of the agents' steps is
// the script's and no sleep decides it.
type Progress struct {
	mu            sync.Mutex
	started, done int
	answers       int
	ended         bool
	changed       chan struct{} // closed and replaced when either count moves, or the agent ends
}

// Answered counts an answer the agent has given. A nil Progress counts nothing.
func (p *Progress) Answered() {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.answers++
	close(p.changed)
	p.changed = make(chan struct{})
	p.mu.Unlock()
}

// WaitSteps returns when the agent has taken at least n steps (calls started and answers given), or
// when ctx ends.
func (p *Progress) WaitSteps(ctx context.Context, n int) {
	for {
		p.mu.Lock()
		ok, changed := p.started+p.answers >= n, p.changed
		p.mu.Unlock()
		if ok {
			return
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return
		}
	}
}

// HoldExec holds the carrying out of a step's call until what its gate names for that moment has
// happened: the agent that started it (parent) having taken as many steps as the gate says.
func HoldExec(ctx context.Context, g scenario.Gate, parent *Progress, ancestors []*Progress) {
	if g.ExecParentSteps > 0 && parent != nil {
		parent.WaitSteps(ctx, g.ExecParentSteps)
	}
	for i, n := range g.ExecAncestorSteps {
		if n > 0 && i < len(ancestors) && ancestors[i] != nil {
			ancestors[i].WaitSteps(ctx, n)
		}
	}
}

// End says the agent has ended. A nil Progress ends nothing.
func (p *Progress) End() {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.ended = true
	close(p.changed)
	p.changed = make(chan struct{})
	p.mu.Unlock()
}

// WaitEnded returns when the agent has ended, or when ctx ends.
func (p *Progress) WaitEnded(ctx context.Context) {
	for {
		p.mu.Lock()
		ok, changed := p.ended, p.changed
		p.mu.Unlock()
		if ok {
			return
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return
		}
	}
}

// NewProgress is an agent that has done nothing yet.
func NewProgress() *Progress { return &Progress{changed: make(chan struct{})} }

// Started is how many calls the agent has started. A nil Progress has none.
func (p *Progress) Started() int {
	if p == nil {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.started
}

// Move counts calls started and finished. A nil Progress counts nothing.
func (p *Progress) Move(started, done int) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.started += started
	p.done += done
	close(p.changed)
	p.changed = make(chan struct{})
	p.mu.Unlock()
}

// Wait returns when at least started calls have been started and done finished, or
// when ctx ends.
func (p *Progress) Wait(ctx context.Context, started, done int) {
	for {
		p.mu.Lock()
		ok, changed := p.started >= started && p.done >= done, p.changed
		p.mu.Unlock()
		if ok {
			return
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return
		}
	}
}

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
