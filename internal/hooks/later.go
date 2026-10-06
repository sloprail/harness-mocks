package hooks

import (
	"context"
	"sync"
)

// Later runs the hooks a config marks to run in the background (async), one after another in the
// order they were started, so what they log is in a fixed order, and keeps what they printed until
// the agent is next told of it. The agent does not wait for them, and what they add reaches it at
// the next safe point: a hook started when the agent had begun `step` calls is delivered once the
// agent has gone through the call after it (Due), or when its turn would end, whichever comes
// first. Delivery waits for the hook to have finished: an event, not a time.
type Later struct {
	mu   sync.Mutex
	last chan struct{} // closed when the run started last has finished
	runs []*laterRun
}

type laterRun struct {
	event string
	step  int
	done  chan struct{}
	outs  []Outcome
}

// Delivered is what the hooks of one event, started in the background, printed.
type Delivered struct {
	Event    string
	Outcomes []Outcome
}

// Start runs cmds in the background once the runs started before it have finished. step is how many
// of its calls the agent had begun.
func (l *Later) Start(ctx context.Context, event string, step int, cmds []Command, stdin []byte, rt Runtime) {
	r := &laterRun{event: event, step: step, done: make(chan struct{})}
	l.mu.Lock()
	prev := l.last
	l.last = r.done
	l.runs = append(l.runs, r)
	l.mu.Unlock()
	go func() {
		defer close(r.done)
		if prev != nil {
			<-prev
		}
		r.outs = RunAll(ctx, cmds, stdin, rt)
	}()
}

// Due is what the hooks that are due printed, in the order they were started, once each has finished:
// those started before the agent began its step-th call, or all of them when the turn ends.
func (l *Later) Due(step int, endOfTurn bool) (out []Delivered) {
	l.mu.Lock()
	var due, keep []*laterRun
	for _, r := range l.runs {
		if endOfTurn || r.step < step {
			due = append(due, r)
		} else {
			keep = append(keep, r)
		}
	}
	l.runs = keep
	l.mu.Unlock()
	for _, r := range due {
		<-r.done
		out = append(out, Delivered{Event: r.event, Outcomes: r.outs})
	}
	return out
}

// Wait returns when every hook started in the background has finished.
func (l *Later) Wait() {
	l.mu.Lock()
	last := l.last
	l.mu.Unlock()
	if last != nil {
		<-last
	}
}
