package subagents

import (
	"context"
	"sync"

	"github.com/sloprail/harness-mocks/internal/scenario"
)

// Progress is how far an agent has got through its tool calls: how many it has
// started and how many it has finished. What another agent's script says it must
// wait for (scenario.Gate) is read against it, so the order of the agents' steps is
// the script's and no sleep decides it.
type Progress struct {
	mu            sync.Mutex
	started, done int
	executed      int // calls carried out (begun to run, or answered with no run)
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

// HoldAncestors holds an agent's step back until the agents above the one that started it have finished
// as many calls as its gate says.
func HoldAncestors(ctx context.Context, g scenario.Gate, ancestors []*Progress) {
	for i, n := range g.AncestorDone {
		if n > 0 && i < len(ancestors) && ancestors[i] != nil {
			ancestors[i].Wait(ctx, 0, n)
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

// Executed counts a call the agent has begun to carry out: its own frames are written. A nil Progress counts nothing.
func (p *Progress) Executed() {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.executed++
	close(p.changed)
	p.changed = make(chan struct{})
	p.mu.Unlock()
}

// WaitExecuted returns when at least n calls of the agent have been carried out, or when ctx ends.
func (p *Progress) WaitExecuted(ctx context.Context, n int) {
	for {
		p.mu.Lock()
		ok, changed := p.executed >= n, p.changed
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
