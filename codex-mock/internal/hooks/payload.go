package hooks

import "encoding/json"

// Common is what every hook payload carries, whatever the event.
type Common struct {
	SessionID      string
	TranscriptPath string
	Cwd            string
	Model          string
	PermissionMode string
	// AgentID and AgentType name the sub-agent a hook fires for; empty for the
	// main thread, whose payloads name neither (recorded: runs/background-agent).
	AgentID, AgentType string
}

// Payload is the JSON object a command hook reads on stdin: the common
// fields, the event's name and the event's own fields (turn_id, tool_name,
// tool_input, prompt, source, …).
//
// sr:docs https://developers.openai.com/codex/hooks#common-input-fields
// sr:provides hook-common-payload/codex
func Payload(c Common, ev Event, own map[string]any) []byte {
	p := map[string]any{
		"session_id":      c.SessionID,
		"transcript_path": c.TranscriptPath,
		"cwd":             c.Cwd,
		"hook_event_name": string(ev),
		"model":           c.Model,
		"permission_mode": c.PermissionMode,
	}
	if c.AgentID != "" {
		p["agent_id"], p["agent_type"] = c.AgentID, c.AgentType
	}
	if ev == SessionEnd { // recorded: a session-end payload names neither model nor permission mode
		delete(p, "model")
		delete(p, "permission_mode")
	}
	if ev == PreCompact || ev == PostCompact { // recorded: a compaction's payload names no permission mode
		delete(p, "permission_mode")
	}
	for k, v := range own {
		p[k] = v
	}
	b, _ := json.Marshal(p)
	return b
}
