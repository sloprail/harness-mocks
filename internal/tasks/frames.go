package tasks

// Observer is told what a run's stream announces of a task: that the set of
// running background tasks changed, that a task started, how its status was
// updated when it ended, and its end notification. How each is written is the
// harness's.
type Observer interface {
	// Changed is the background tasks running now, told when one joins or leaves
	// them: before a task's start is announced, and before its end is.
	Changed(running []*Task)
	Started(*Task)
	// Updated is the end status the task's update reports: "killed" for a
	// command the harness ended, else the task's own status.
	Updated(t *Task, status string)
	Notified(*Task)
}

// Announce reports that a task started (a background task or a sub-agent): first
// the running set it joins, when it is one of the registry's background tasks (a
// foreground sub-agent is not), then the start.
func Announce(r *Registry, t *Task, o Observer) {
	if r.has(t.ID) {
		o.Changed(r.Running())
	}
	o.Started(t)
}

// Conclude reports a task's end: the running set without it, when it is one of the
// registry's background tasks, then its status update, then its notification.
func Conclude(r *Registry, t *Task, o Observer) {
	if r.has(t.ID) {
		var rest []*Task
		for _, other := range r.Running() {
			if other.ID != t.ID {
				rest = append(rest, other)
			}
		}
		o.Changed(rest)
	}
	updated := string(t.Status())
	if t.Killed() {
		updated = "killed"
	}
	o.Updated(t, updated)
	o.Notified(t)
}
