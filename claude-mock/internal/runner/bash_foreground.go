package runner

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/sloprail/harness-mocks/claude-mock/internal/toolexec"
)

// foregroundTaskAfter is how long a foreground Bash command of the main agent runs before real Claude
// Code makes a task of it: task_started {is_backgrounded:false, task_type:"local_bash"} when it
// crosses this, task_notification when it ends. Recorded: a `sleep 3` made one, a `sleep 2.9`, 2.5 and
// 2 did not (snapshots/runs/bash-long-foreground and the hook-timeout run's `sleep 6`).
const foregroundTaskAfter = 3 * time.Second

// bashDescription is what a Bash call's task frames call it: its description, else its command.
func bashDescription(call pendingToolUse) string {
	var in struct{ Command, Description string }
	_ = json.Unmarshal(call.ToolInput, &in)
	if in.Description != "" {
		return in.Description
	}
	return in.Command
}

// slowBashFrames streams the task frames of a foreground Bash command of the main agent that outlasts
// foregroundTaskAfter, and returns what ends them (a failed command's notification reads "failed": that
// status is not measured). A command that ends sooner leaves none.
func slowBashFrames(cfg Config, call pendingToolUse) func(toolexec.Result) {
	if cfg.AgentID != "" {
		return func(toolexec.Result) {}
	}
	var mu sync.Mutex
	var started, done bool
	id, desc := "b"+randomID(8), bashDescription(call)
	timer := time.AfterFunc(foregroundTaskAfter, func() {
		mu.Lock()
		defer mu.Unlock()
		if !done {
			started = true
			writeTaskStarted(cfg, taskStart{ID: id, ToolUseID: call.ToolUseID, Description: desc, TaskType: "local_bash"})
		}
	})
	return func(res toolexec.Result) {
		timer.Stop()
		mu.Lock()
		defer mu.Unlock()
		done = true
		if started {
			writeTaskNotification(cfg, taskNote{ID: id, ToolUseID: call.ToolUseID, Status: map[bool]string{false: "completed", true: "failed"}[res.IsError], Summary: desc})
		}
	}
}
