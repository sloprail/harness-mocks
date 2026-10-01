package hooks

import "encoding/json"

// Common is what every payload carries beside its event's own fields.
type Common struct {
	SessionID string
	// TranscriptPath is the conversation's transcript file; "" is none yet.
	TranscriptPath string
	// Dir is the workspace root.
	Dir string
	// Version is the Cursor version the payloads report.
	Version string
}

// Tool names a tool call as the hooks see it.
type Tool struct {
	// Name is the tool's name in hooks: Shell, Read or Write.
	Name string
	// Input is the call's input as hooks see it.
	Input map[string]any
	// UseID identifies the call.
	UseID string
}

// Payload is the JSON a hook of the event reads on stdin: the common fields,
// then the event's own. The account's email is never known to the mock.
//
// sr:docs https://cursor.com/docs/hooks#common-schema
func (c Common) Payload(e Event, own map[string]any) []byte {
	p := map[string]any{
		"conversation_id": c.SessionID, "generation_id": c.SessionID, "session_id": c.SessionID,
		"model": "default", "hook_event_name": string(e), "cursor_version": c.Version,
		"workspace_roots": []string{c.Dir}, "user_email": nil, "transcript_path": nil,
	}
	if c.TranscriptPath != "" {
		p["transcript_path"] = c.TranscriptPath
	}
	for k, v := range own {
		p[k] = v
	}
	b, _ := json.Marshal(p)
	return b
}

// ToolFields are the fields a tool hook carries for its call.
func ToolFields(t Tool) map[string]any {
	return map[string]any{"tool_name": t.Name, "tool_input": t.Input, "tool_use_id": t.UseID, "cwd": ""}
}
