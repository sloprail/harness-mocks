package runner

import "github.com/sloprail/harness-mocks/internal/tasks"

// announce writes the launch's start frames (the running set, task_started) now, as the harness does ahead
// of the result that answers a background launch (recorded: runs/bgagent, bgagent-concurrent-limit).
func (s *subagentRun) announce(bg *backgroundTasks, prompt string) {
	tasks.Announce(bg.Registry, s.frameTask(prompt), frameObserver{s.parent})
	s.startAnnounced = true
}
