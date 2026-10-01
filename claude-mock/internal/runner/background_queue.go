package runner

import (
	"context"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
)

// running is the session's still-running tasks, as a Stop/SubagentStop
// payload's background_tasks lists them.
func (b *backgroundTasks) running() []hooks.BackgroundTask {
	out := []hooks.BackgroundTask{}
	if b == nil {
		return out
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, t := range b.tasks {
		if t.finished() {
			continue
		}
		if t.agent {
			out = append(out, hooks.BackgroundTask{ID: t.id, Type: "subagent", Status: "running", Description: t.description, AgentType: t.agentType})
		} else {
			out = append(out, hooks.BackgroundTask{ID: t.id, Type: "shell", Status: "running", Description: t.description, Command: t.command})
		}
	}
	return out
}

// takeFinished claims, in launch order, every finished task owner launched
// that has not been handed over yet.
func (b *backgroundTasks) takeFinished(owner string) []*backgroundTask {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []*backgroundTask
	for _, t := range b.tasks {
		if t.owner == owner && !t.delivered && t.finished() {
			t.delivered = true
			out = append(out, t)
		}
	}
	return out
}

// agentsRunning reports whether owner has a background Agent still running.
func (b *backgroundTasks) agentsRunning(owner string) bool {
	if b == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, t := range b.tasks {
		if t.owner == owner && t.agent && !t.finished() {
			return true
		}
	}
	return false
}

// awaitAfterTurn is what a `claude -p` session does once a turn has ended and
// Stop let it: it returns the next finished task of owner to hand over as a
// new turn, waiting while owner still has a background Agent running. It
// returns nil when there is nothing left to wait for.
func (b *backgroundTasks) awaitAfterTurn(ctx context.Context, owner string) *backgroundTask {
	for {
		if ts := b.takeFirstFinished(owner); ts != nil {
			return ts
		}
		if !b.agentsRunning(owner) {
			return nil
		}
		select {
		case <-b.changed:
		case <-ctx.Done():
			return nil
		}
	}
}

func (b *backgroundTasks) takeFirstFinished(owner string) *backgroundTask {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, t := range b.tasks {
		if t.owner == owner && !t.delivered && t.finished() {
			t.delivered = true
			return t
		}
	}
	return nil
}
