package runner

import (
	"time"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/internal/tasks"
)

// running is the session's still-running tasks, as a Stop/SubagentStop
// payload's background_tasks lists them: a command as a "shell" entry, an agent
// as a "subagent" one.
func (b *backgroundTasks) running() []hooks.BackgroundTask {
	out := []hooks.BackgroundTask{}
	if b == nil {
		return out
	}
	return backgroundTaskList(b.Running())
}

// backgroundTaskList is the payload entries for tasks.
func backgroundTaskList(running []*tasks.Task) []hooks.BackgroundTask {
	out := []hooks.BackgroundTask{}
	for _, t := range running {
		if t.Kind == tasks.Agent {
			out = append(out, hooks.BackgroundTask{ID: t.ID, Type: "subagent", Status: "running", Description: t.Description, AgentType: t.AgentType})
		} else {
			out = append(out, hooks.BackgroundTask{ID: t.ID, Type: "shell", Status: "running", Description: t.Description, Command: t.Command})
		}
	}
	return out
}

// printReapGrace is how long after the final result a `claude -p` run leaves its
// background shells before ending them, so that one finishing right after the
// result still delivers its output (headless#background-tasks-at-exit).
const printReapGrace = 5 * time.Second
