package runner

import "time"

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
