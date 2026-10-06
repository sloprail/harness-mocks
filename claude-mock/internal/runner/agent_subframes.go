package runner

import (
	"encoding/json"
	"path/filepath"
	"sync/atomic"
	"time"
)

// SubFrames is what a sub-agent's stream frames name beside the message: the
// tool call that started it (parent_tool_use_id), its description
// (task_description) and the count behind its task_progress frames (recorded:
// snapshots/runs/isolated-worktree, fg-subagent-bash).
type SubFrames struct {
	ParentToolUseID, TaskDescription string
	// TaskToolUseID is the call the task's progress frames name, when not the one that started the
	// sub-agent: the message that resumed it (recorded: runs/fgsub-maxturns).
	TaskToolUseID string
	begun         time.Time
	toolUses      atomic.Int64
}

// frameFields adds a sub-agent's frame fields to a stamped assistant or user frame.
func (s *SubFrames) frameFields(cfg Config, m map[string]any) {
	m["parent_tool_use_id"] = s.ParentToolUseID
	m["subagent_type"] = cfg.AgentType
	m["task_description"] = s.TaskDescription
}

// progress streams the task_progress frame that precedes each tool call of a
// sub-agent: what it is about to do, the calls so far and how long it has run. The
// mock spends no tokens.
func (s *SubFrames) progress(cfg Config, tool string, input json.RawMessage) {
	writeFrame(cfg, map[string]any{
		"type": "system", "subtype": "task_progress", "task_id": cfg.AgentID, "tool_use_id": s.taskToolUseID(),
		"description": progressWhat(tool, input), "subagent_type": cfg.AgentType, "last_tool_name": tool,
		"usage": map[string]any{"total_tokens": 0, "tool_uses": s.toolUses.Add(1), "duration_ms": time.Since(s.begun).Milliseconds()},
	})
}

// progressWhat is what a task_progress frame says of the call about to run, as recorded
// (runs/meta, bgagent, file-tools...): an Agent call its own description, a Bash "Running "
// and its description, a Read "Reading " and an Edit "Editing " and the file's name; any other
// tool "Running " and its description, else its name.
func progressWhat(tool string, input json.RawMessage) string {
	var in struct {
		Description string `json:"description"`
		FilePath    string `json:"file_path"`
	}
	_ = json.Unmarshal(input, &in)
	switch {
	case tool == "Agent" && in.Description != "":
		return in.Description
	case tool == "Read" && in.FilePath != "":
		return "Reading " + filepath.Base(in.FilePath)
	case tool == "Edit" && in.FilePath != "":
		return "Editing " + filepath.Base(in.FilePath)
	case in.Description != "":
		return "Running " + in.Description
	}
	return "Running " + tool
}

// announce streams the sub-agent's first frame: the prompt it was given, as a user message.
func (s *SubFrames) announce(cfg Config, prompt string) {
	s.begun = time.Now()
	line, err := json.Marshal(map[string]any{"type": "user", "message": map[string]any{"role": "user",
		"content": []any{map[string]any{"type": "text", "text": prompt}}}})
	if err == nil {
		writeStreamLine(cfg, line)
	}
}

func (s *SubFrames) taskToolUseID() string {
	if s.TaskToolUseID != "" {
		return s.TaskToolUseID
	}
	return s.ParentToolUseID
}
