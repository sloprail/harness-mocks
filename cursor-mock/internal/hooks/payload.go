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
	// Model is the model the run was started with (--model); "" is the default.
	Model string
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
// sr:provides hook-common-payload/cursor
// sr:docs https://cursor.com/docs/hooks#common-schema
func (c Common) Payload(e Event, own map[string]any) []byte {
	if e == WorkspaceOpen { // an app event, outside any session: no session, model or transcript
		b, _ := json.Marshal(map[string]any{"hook_event_name": string(e), "cursor_version": c.Version, "workspace_roots": []string{c.Dir}, "user_email": nil})
		return b
	}
	p := map[string]any{
		"conversation_id": c.SessionID, "generation_id": c.SessionID, "session_id": c.SessionID,
		"model": "default", "hook_event_name": string(e), "cursor_version": c.Version,
		"workspace_roots": []string{c.Dir}, "user_email": nil, "transcript_path": nil,
	}
	if c.TranscriptPath != "" {
		p["transcript_path"] = c.TranscriptPath
	}
	if e == SessionEnd && c.Model != "" && c.Model != "auto" { // the end of a run names the model it was started with (recorded: runs/hook-matchers-thought)
		p["model"] = c.Model
	}
	for k, v := range own {
		p[k] = v
	}
	b, _ := json.Marshal(p)
	return b
}

// ToolFields are the fields a tool hook carries for its call. Only a Shell
// call carries a cwd (recorded empty: the command ran from the workspace
// root); the Read, Write and Task events have none.
func ToolFields(t Tool) map[string]any {
	f := map[string]any{"tool_name": t.Name, "tool_input": t.Input, "tool_use_id": t.UseID}
	if t.Name == "Shell" {
		f["cwd"] = ""
	}
	return f
}
