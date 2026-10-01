package tasks

// Observer is told what a run's stream announces of a task: that it started,
// how its status was updated when it ended, and its end notification. How each
// is written is the harness's.
type Observer interface {
	Started(*Task)
	// Updated is the end status the task's update reports: "killed" for a
	// command the harness ended, else the task's own status.
	Updated(t *Task, status string)
	Notified(*Task)
}

// Announce reports that a task started (a background task or a sub-agent).
func Announce(t *Task, o Observer) { o.Started(t) }

// Conclude reports a task's end: its status update, then its notification.
//
// sr:capability task-stream-frames
func Conclude(t *Task, o Observer) {
	updated := string(t.Status())
	if t.Killed() {
		updated = "killed"
	}
	o.Updated(t, updated)
	o.Notified(t)
}
