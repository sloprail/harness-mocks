package runner

import "encoding/json"

// toolDefaults are the parameters the harness fills into a tool call's input when the model left them
// out, by tool (recorded: snapshots/runs/file-tools: the Edit the model sent without replace_all is
// streamed, hooked and written with replace_all false, and its wire input is what the model sent).
var toolDefaults = map[string]map[string]any{"Edit": {"replace_all": false}}

// withToolDefaults is an assistant line with the defaults of its tool calls filled in, as the
// transcript, the hooks and the tool see it, and the same as it is streamed, which also names the
// inputs as the model sent them (stampFrame's wire_tool_inputs). A line that has none to fill is both,
// unchanged, and the first call's input as it then is.
func withToolDefaults(cfg Config, line []byte) (visible, streamed []byte, input json.RawMessage) {
	visible, streamed = fillToolDefaults(cfg, line)
	_, _, input = extractFirstToolUseWithID(visible)
	return visible, streamed, input
}

func fillToolDefaults(cfg Config, line []byte) (visible, streamed []byte) {
	var m map[string]any
	if json.Unmarshal(line, &m) != nil || m["type"] != "assistant" {
		return line, line
	}
	msg, _ := m["message"].(map[string]any)
	blocks, _ := msg["content"].([]any)
	wire := copyInputs(wireToolInputs(m)) // before the defaults go into the very maps it holds
	filled := false
	for _, b := range blocks {
		block, _ := b.(map[string]any)
		in, _ := block["input"].(map[string]any)
		name, _ := block["name"].(string)
		if block["type"] != "tool_use" || in == nil {
			continue
		}
		for k, v := range toolDefaults[name] {
			if _, ok := in[k]; !ok {
				in[k], filled = v, true
			}
		}
	}
	if !filled {
		return line, line
	}
	v, err := marshalRecord(m)
	if err != nil {
		return line, line
	}
	if cfg.AgentID != "" { // only the main agent's frames carry the inputs as the model sent them
		return v, v
	}
	m["wire_tool_inputs"] = wire
	s, err := marshalRecord(m)
	if err != nil {
		return line, line
	}
	return v, s
}

// copyInputs is wire with each input copied (one level: the defaults are top-level parameters).
func copyInputs(wire map[string]any) map[string]any {
	if wire == nil {
		return nil
	}
	out := make(map[string]any, len(wire))
	for id, in := range wire {
		if m, ok := in.(map[string]any); ok {
			c := make(map[string]any, len(m))
			for k, v := range m {
				c[k] = v
			}
			in = c
		}
		out[id] = in
	}
	return out
}
