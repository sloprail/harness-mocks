package tasks

// Adopt hands the background agents from still has running over to to: in a
// non-interactive run a sub-agent that launched one does not wait for it, so
// once it has ended the agent reports to whoever launched the sub-agent, to
// the main conversation at the top.
func (r *Registry) Adopt(from, to string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.tasks {
		if t.Owner == from && t.Kind == Agent && !t.Finished() {
			t.Owner = to
		}
	}
}
