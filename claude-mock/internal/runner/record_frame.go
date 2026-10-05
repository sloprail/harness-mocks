package runner

import (
	"encoding/json"
	"time"
)

// stampFrame is the stream frame of an assistant or user record as real Claude
// Code writes it: beside the message, the session it belongs to, the tool
// call of the sub-agent it comes from (null on the main agent's) and when it was
// written, unless the
// scenario's own record already names them (recorded: snapshots/runs/bashfail).
func stampFrame(cfg Config, line []byte) []byte {
	var m map[string]any
	if json.Unmarshal(line, &m) != nil || (m["type"] != "assistant" && m["type"] != "user") {
		return line
	}
	if _, ok := m["session_id"]; !ok {
		m["session_id"] = cfg.SessionID
	}
	if _, ok := m["parent_tool_use_id"]; !ok {
		m["parent_tool_use_id"] = nil
		if cfg.AgentID != "" { // a sub-agent's frame names the call that started it
			cfg.frameFields(cfg, m)
		}
	}
	if _, ok := m["timestamp"]; !ok { // when the frame was written (recorded: every assistant and user frame)
		m["timestamp"] = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	}
	// the main agent's tool calls are also given as the inputs that went over the
	// wire, by call id; a sub-agent's frames have none
	if m["type"] == "assistant" && m["parent_tool_use_id"] == nil {
		if wire := wireToolInputs(m); wire != nil {
			if _, ok := m["wire_tool_inputs"]; !ok {
				m["wire_tool_inputs"] = wire
			}
		}
	}
	if b, err := marshalRecord(m); err == nil {
		return b
	}
	return line
}

// wireToolInputs are the inputs of the tool_use blocks of an assistant frame, by
// the call's id; nil when it has none.
func wireToolInputs(frame map[string]any) map[string]any {
	msg, _ := frame["message"].(map[string]any)
	blocks, _ := msg["content"].([]any)
	var wire map[string]any
	for _, b := range blocks {
		block, _ := b.(map[string]any)
		if id, _ := block["id"].(string); block["type"] == "tool_use" && id != "" {
			if wire == nil {
				wire = map[string]any{}
			}
			wire[id] = block["input"]
		}
	}
	return wire
}

// isMessageFrame is whether a stream line is a frame a sub-agent streams: a user
// message (its prompt, a tool's result) or an assistant message that calls a
// tool. Its final answer does not stream: the task's notification carries it
// (recorded: snapshots/runs/isolated-worktree).
func isMessageFrame(line []byte) bool {
	var f struct {
		Type    string `json:"type"`
		Message struct {
			Content []struct {
				Type string `json:"type"`
			} `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal(line, &f) != nil {
		return false
	}
	if f.Type == "user" {
		return true
	}
	for _, b := range f.Message.Content {
		if f.Type == "assistant" && b.Type == "tool_use" {
			return true
		}
	}
	return false
}
