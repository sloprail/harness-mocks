package subagents

import "context"

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
