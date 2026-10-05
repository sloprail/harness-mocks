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
	changed       chan struct{} // closed and replaced when either count moves
}

// NewProgress is an agent that has done nothing yet.
func NewProgress() *Progress { return &Progress{changed: make(chan struct{})} }

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

// SpawnLog is the ids of the sub-agents an agent has started, in order: the position
// a script's gate names a sub-agent by.
type SpawnLog struct {
	mu  sync.Mutex
	ids []string
}

// Add records a sub-agent the agent has started. A nil SpawnLog records nothing.
func (l *SpawnLog) Add(id string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.ids = append(l.ids, id)
	l.mu.Unlock()
}

func (l *SpawnLog) at(k int) (string, bool) {
	if l == nil {
		return "", false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if k < 0 || k >= len(l.ids) {
		return "", false
	}
	return l.ids[k], true
}

// Hold holds an agent's step back until what its script's gate names has happened: the
// sub-agents it started (spawned) that must have ended, and how far the agent that
// started it (parent, nil for the session's own) must have got. A gate that names what
// does not exist is the script's mistake: it is returned as a problem to report, not
// waited for.
func Hold(ctx context.Context, g scenario.Gate, reg *tasks.Registry, spawned *SpawnLog, parent *Progress) (problems []string) {
	if g.ParentStarted > 0 || g.ParentDone > 0 {
		if parent == nil {
			problems = append(problems, "a gate waits for the agent that started this one, and this one was started by none")
		} else {
			parent.Wait(ctx, g.ParentStarted, g.ParentDone)
		}
	}
	for _, k := range g.Ended {
		id, ok := spawned.at(k)
		if !ok {
			problems = append(problems, fmt.Sprintf("a gate waits for sub-agent %d, which this agent has not started", k))
			continue
		}
		awaitEnd(ctx, reg, id)
	}
	return problems
}

// awaitEnd waits for the sub-agent's task to end, or for ctx to.
func awaitEnd(ctx context.Context, reg *tasks.Registry, id string) {
	for reg.Find(id) == nil { // its task is registered once its spawn has been answered
		select {
		case <-ctx.Done():
			return
		default:
		}
	}
	select {
	case <-reg.Find(id).Done():
	case <-ctx.Done():
	}
}
