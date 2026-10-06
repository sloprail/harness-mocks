package tasks

import "context"

// Await is the task with this id once the registry holds it (a sub-agent's task is registered when
// its spawn has been answered), or nil when ctx ends first. It waits for the registration, which
// wakes it: no polling.
func (r *Registry) Await(ctx context.Context, id string) *Task {
	for {
		r.mu.Lock()
		var found *Task
		for _, t := range r.tasks {
			if t.ID == id {
				found = t
				break
			}
		}
		added := r.added
		r.mu.Unlock()
		if found != nil {
			return found
		}
		select {
		case <-added:
		case <-ctx.Done():
			return nil
		}
	}
}
