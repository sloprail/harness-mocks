package subagents

import (
	"context"
	"sync"
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
