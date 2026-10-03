package tasks

import (
	"context"
	"time"
)

// StartYielding starts a command as StartCommand does and waits up to wait for
// it: ended reports whether it ended within that time. A command that has not
// is left running, for the caller to answer with a receipt (background-bash:
// what a harness does of a command it lets go on after a yield time).
//
// sr:capability background-bash
func (r *Registry) StartYielding(ctx context.Context, t *Task, s CommandSpec, wait time.Duration) (ended bool, err error) {
	if err := r.StartCommand(t, s); err != nil {
		return false, err
	}
	select {
	case <-t.Done():
		return true, nil
	case <-time.After(wait):
	case <-ctx.Done():
	}
	return false, nil
}

// AwaitEnds waits, until ctx ends, for each of launched to end, and returns
// the ones that did, in launch order: what a non-interactive run does before
// it finishes when it waits for its background commands.
//
// sr:capability background-bash
func AwaitEnds(ctx context.Context, launched []*Task) []*Task {
	var ended []*Task
	for _, t := range launched {
		select {
		case <-t.Done():
			ended = append(ended, t)
		case <-ctx.Done():
		}
	}
	return ended
}

// AfterHookFires reports whether the after-tool hooks fire for a call whose
// command was let go: not while it still runs, the end of the command being
// what they would report.
//
// sr:capability background-bash
func AfterHookFires(stillRunning bool) bool { return !stillRunning }
