package events

// AgentState is how a sub-agent stands in a collab_tool_call item.
type AgentState struct {
	Status  string `json:"status"`
	Message any    `json:"message"`
}

// CollabStarted reports a call that manages sub-agents (spawn_agent, wait)
// that began; the id pairs it with its end. prompt is nil when the call has
// none.
func (s *Stream) CollabStarted(tool, sender string, receivers []string, prompt any) string {
	id := s.newID()
	s.emit(map[string]any{"type": "item.started", "item": collabItem(id, tool, sender, receivers, prompt,
		map[string]AgentState{}, "in_progress")})
	return id
}

// CollabCompleted reports how that call ended: the sub-agents it concerns and
// how each stands (recorded: runs/subagent-lifecycle-hooks). A sub-agent is
// announced by the spawn call's item completing with its thread pending, and its
// status and end are told by the item completing the wait, as its state with its
// answer as the message; there is no frame of a task of its own (recorded:
// runs/task-stream-frames).
// sr:provides task-stream-frames/codex
func (s *Stream) CollabCompleted(id, tool, sender string, receivers []string, prompt any, states map[string]AgentState) {
	s.emit(map[string]any{"type": "item.completed", "item": collabItem(id, tool, sender, receivers, prompt, states, "completed")})
}

func collabItem(id, tool, sender string, receivers []string, prompt any, states map[string]AgentState, status string) map[string]any {
	if receivers == nil {
		receivers = []string{}
	}
	return map[string]any{"id": id, "type": "collab_tool_call", "tool": tool, "sender_thread_id": sender,
		"receiver_thread_ids": receivers, "prompt": prompt, "agents_states": states, "status": status}
}
