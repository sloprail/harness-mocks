package tasks

import (
	"context"
	"sync"
)

// Registry is a session's background tasks. Sub-agents share their parent's:
// what is listed as running is the whole session's, while a finished task is
// handed to the agent that launched it.
type Registry struct {
	mu      sync.Mutex
	tasks   []*Task
	changed chan struct{} // signalled (non-blocking) whenever a task finishes
	wg      sync.WaitGroup
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewRegistry is an empty registry.
func NewRegistry() *Registry {
	ctx, cancel := context.WithCancel(context.Background())
	return &Registry{changed: make(chan struct{}, 1), ctx: ctx, cancel: cancel}
}

// Context is cancelled when the registry shuts down; background sub-agents run
// under it.
func (r *Registry) Context() context.Context { return r.ctx }

// has reports whether the registry holds an unfinished task with this id; a nil
// registry holds none.
func (r *Registry) has(id string) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.tasks {
		if t.ID == id && !t.Finished() {
			return true
		}
	}
	return false
}

// Find is the task with this id, finished or not; nil when the registry holds none.
func (r *Registry) Find(id string) *Task {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.tasks {
		if t.ID == id {
			return t
		}
	}
	return nil
}

// Add registers a task, in launch order.
func (r *Registry) Add(t *Task) {
	r.mu.Lock()
	r.tasks = append(r.tasks, t)
	r.mu.Unlock()
}

// Go runs fn concurrently, tracked so Shutdown waits for it.
func (r *Registry) Go(fn func()) {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		fn()
	}()
}

// Finish marks t ended and wakes anyone waiting for a task to finish.
func (r *Registry) Finish(t *Task) {
	close(t.done)
	select {
	case r.changed <- struct{}{}:
	default:
	}
}

// Running is the session's still-running tasks, in launch order.
func (r *Registry) Running() []*Task {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*Task
	for _, t := range r.tasks {
		if !t.Finished() {
			out = append(out, t)
		}
	}
	return out
}

// AgentsRunning reports whether owner has a background agent still running.
func (r *Registry) AgentsRunning(owner string) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.tasks {
		if t.Owner == owner && t.Kind == Agent && !t.Finished() {
			return true
		}
	}
	return false
}
