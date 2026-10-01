package tasks

import "context"

// TakeFinished claims, in launch order, every finished task owner launched that
// has not been handed over yet: what an owner that is still working is told
// inside its turn, after its next tool result.
//
// sr:capability task-notifications
func (r *Registry) TakeFinished(owner string) []*Task {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*Task
	for _, t := range r.tasks {
		if t.Owner == owner && !t.delivered && t.Finished() {
			t.delivered = true
			out = append(out, t)
		}
	}
	return out
}

// AwaitAfterTurn is what a non-interactive session does once a turn has ended:
// it returns the next finished task of owner, to start a further turn, waiting
// while owner still has a background agent running. It returns nil when there
// is nothing left to wait for.
//
// sr:capability print-waits-for-background-agents
func (r *Registry) AwaitAfterTurn(ctx context.Context, owner string) *Task {
	for {
		if t := r.takeFirstFinished(owner); t != nil {
			return t
		}
		if !r.AgentsRunning(owner) {
			return nil
		}
		select {
		case <-r.changed:
		case <-ctx.Done():
			return nil
		}
	}
}

// NextTurn is the next task to hand owner as a turn of its own that accept lets
// through: a notification a prompt hook refuses starts no turn, and the next
// finished task is handed over instead.
func (r *Registry) NextTurn(ctx context.Context, owner string, accept func(*Task) bool) *Task {
	for t := r.AwaitAfterTurn(ctx, owner); t != nil; t = r.AwaitAfterTurn(ctx, owner) {
		if accept(t) {
			return t
		}
	}
	return nil
}

func (r *Registry) takeFirstFinished(owner string) *Task {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.tasks {
		if t.Owner == owner && !t.delivered && t.Finished() {
			t.delivered = true
			return t
		}
	}
	return nil
}
