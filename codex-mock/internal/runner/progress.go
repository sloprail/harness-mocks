package runner

import (
	"context"
	"fmt"
	"sync"

	"github.com/sloprail/harness-mocks/internal/scenario"
)

// progress is how far an agent has got through its tool calls: how many it has
// started and how many it has finished. What another agent's script says it must
// wait for (scenario.Gate) is read against it, so the order of the agents' steps is
// the script's and no sleep decides it.
type progress struct {
	mu            sync.Mutex
	started, done int
	changed       chan struct{} // closed and replaced when either count moves
}

func newProgress() *progress { return &progress{changed: make(chan struct{})} }

func (p *progress) move(started, done int) {
	if p == nil { // a host built without one (a test)
		return
	}
	p.mu.Lock()
	p.started += started
	p.done += done
	close(p.changed)
	p.changed = make(chan struct{})
	p.mu.Unlock()
}

// wait returns when at least started calls have been started and done finished, or
// when ctx ends.
func (p *progress) wait(ctx context.Context, started, done int) {
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

// spawnLog is the ids of the sub-agents an agent has started, in order: the position
// a script's gate names a sub-agent by.
type spawnLog struct {
	mu  sync.Mutex
	ids []string
}

func (l *spawnLog) add(id string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.ids = append(l.ids, id)
	l.mu.Unlock()
}

func (l *spawnLog) at(k int) (string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if k < 0 || k >= len(l.ids) {
		return "", false
	}
	return l.ids[k], true
}

// Gate holds the agent's step back until what its script's gate names has happened:
// the sub-agents it started that must have ended, and how far the agent that started
// it must have got. A gate that names what does not exist is the script's mistake and
// is reported loudly, not waited for.
func (h *state) Gate(ctx context.Context, g scenario.Gate) {
	if g.ParentStarted > 0 || g.ParentDone > 0 {
		if h.parent == nil {
			fmt.Fprintf(h.cfg.Stderr, "ERROR codex_mock: a gate waits for the agent that started this one, and this one was started by none\n")
		} else {
			h.parent.wait(ctx, g.ParentStarted, g.ParentDone)
		}
	}
	for _, k := range g.Ended {
		id, ok := h.spawned.at(k)
		if !ok {
			fmt.Fprintf(h.cfg.Stderr, "ERROR codex_mock: a gate waits for sub-agent %d, which this agent has not started\n", k)
			continue
		}
		h.awaitEnd(ctx, id)
	}
}

// awaitEnd waits for the sub-agent's task to end, or for ctx to.
func (h *state) awaitEnd(ctx context.Context, id string) {
	for h.bg.Find(id) == nil { // its task is registered once its spawn has been answered
		select {
		case <-ctx.Done():
			return
		default:
		}
	}
	select {
	case <-h.bg.Find(id).Done():
	case <-ctx.Done():
	}
}
