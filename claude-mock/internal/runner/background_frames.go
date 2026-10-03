package runner

import (
	"time"

	"github.com/sloprail/harness-mocks/internal/tasks"
)

// frameObserver writes Claude Code's stream frames for what the tasks core
// reports of a task: a task_started frame from the taskStart the launcher
// attached (Meta), a task_updated frame, and a task_notification frame. A
// command killed at the end of a session is "killed", then "stopped" with its
// description as summary (F:bgbash); one that finished carries the
// notification's summary (F:midturn); a sub-agent's is its result text or
// failure, with its usage.
//
// sr:provides task-stream-frames/claude
type frameObserver struct{ cfg Config }

// Changed streams the background tasks running now (recorded: bgbash, midturn,
// bgagent, fg-subagent-bash): each as {task_id, task_type, description}, the list
// empty once the last has ended.
func (o frameObserver) Changed(running []*tasks.Task) {
	list := []map[string]any{}
	for _, t := range running {
		taskType := "local_bash"
		if t.Kind == tasks.Agent {
			taskType = "local_agent"
		}
		list = append(list, map[string]any{"task_id": t.ID, "task_type": taskType, "description": t.Description})
	}
	writeFrame(o.cfg, map[string]any{"type": "system", "subtype": "background_tasks_changed", "tasks": list})
}

func (o frameObserver) Started(t *tasks.Task) {
	if s, ok := t.Meta.(taskStart); ok {
		writeTaskStarted(o.cfg, s)
	}
}

func (o frameObserver) Updated(t *tasks.Task, status string) { writeTaskUpdated(o.cfg, t.ID, status) }

func (o frameObserver) Notified(t *tasks.Task) {
	n := taskNote{ID: t.ID, ToolUseID: t.ToolUseID, Status: string(t.Status()), OutputFile: t.OutputFile, Summary: taskSummary(t)}
	switch {
	case t.Kind == tasks.Agent:
		n.Summary = t.Result
		if s, ok := t.Meta.(taskStart); ok && !s.Backgrounded {
			n.Summary = scannedSummary(t.Result) // a foreground sub-agent's report as its parent reads it
		}
		if t.Failure != "" {
			n.Summary = t.Failure
		}
		n.Usage = map[string]any{"total_tokens": 0, "tool_uses": t.ToolUses, "duration_ms": t.DurationMs}
	case t.Killed():
		n.Summary = t.Description
	}
	writeTaskNotification(o.cfg, n)
}

// The task frames a stream-json run carries for a task (a sub-agent, a Bash
// command, backgrounded or not): task_started when it begins, task_updated
// with its end status, and task_notification with its outcome. Only these
// write them.

// taskStart is a task_started frame's content. SubagentType, SpawnDepth and
// Prompt are a sub-agent's (task type local_agent); OwnedBySubagent marks a
// Bash command a sub-agent ran.
type taskStart struct {
	ID, ToolUseID, Description, TaskType string
	Backgrounded, OwnedBySubagent        bool
	SubagentType, Prompt                 string
	SpawnDepth                           int
}

func writeTaskStarted(cfg Config, t taskStart) {
	frame := map[string]any{
		"type": "system", "subtype": "task_started", "task_id": t.ID, "tool_use_id": t.ToolUseID,
		"description": t.Description, "is_backgrounded": t.Backgrounded, "task_type": t.TaskType,
	}
	if t.TaskType == "local_agent" {
		frame["subagent_type"], frame["spawn_depth"], frame["prompt"] = t.SubagentType, t.SpawnDepth, t.Prompt
	}
	if t.OwnedBySubagent {
		frame["owned_by_subagent"] = true
	}
	writeFrame(cfg, frame)
}

func writeTaskUpdated(cfg Config, id, status string) {
	writeFrame(cfg, map[string]any{
		"type": "system", "subtype": "task_updated", "task_id": id,
		"patch": map[string]any{"status": status, "end_time": time.Now().UnixMilli()},
	})
}

// taskNote is a task_notification frame's content; Usage is a sub-agent's.
type taskNote struct {
	ID, ToolUseID, Status, OutputFile, Summary string
	Usage                                      map[string]any
}

func writeTaskNotification(cfg Config, n taskNote) {
	frame := map[string]any{
		"type": "system", "subtype": "task_notification", "task_id": n.ID, "tool_use_id": n.ToolUseID,
		"status": n.Status, "output_file": n.OutputFile, "summary": n.Summary,
	}
	if n.Usage != nil {
		frame["usage"] = n.Usage
	}
	writeFrame(cfg, frame)
}
