package runner

import (
	"encoding/json"
	"strconv"
	"strings"
)

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
	filled := nameSpawned(cfg, blocks)    // a script's "spawn:N" is the id the run minted: what the model would have sent
	wire := copyInputs(wireToolInputs(m)) // before the defaults go into the very maps it holds
	for _, b := range blocks {
		block, _ := b.(map[string]any)
		in, _ := block["input"].(map[string]any)
		name, _ := block["name"].(string)
		if block["type"] != "tool_use" || in == nil {
			continue
		}
		if name == "SendMessage" { // the harness fills type, recipient and content (recorded: runs/fgsub-maxturns)
			filled = messageDefaults(in) || filled
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

// spawnRef is how a script names the sub-agent at a position among those the agent started ("spawn:0"):
// the agent's id is minted by the run, so a script cannot know it.
const spawnRef = "spawn:"

// nameSpawned puts the ids of the sub-agents a SendMessage call names (to, recipient) in place of the
// positions the script gave, and reports whether it changed any.
func nameSpawned(cfg Config, blocks []any) (changed bool) {
	for _, b := range blocks {
		block, _ := b.(map[string]any)
		in, _ := block["input"].(map[string]any)
		if block["name"] != "SendMessage" || in == nil {
			continue
		}
		for _, k := range []string{"to", "recipient"} {
			ref, _ := in[k].(string)
			n, err := strconv.Atoi(strings.TrimPrefix(ref, spawnRef))
			if !strings.HasPrefix(ref, spawnRef) || err != nil {
				continue
			}
			if id, ok := cfg.steps.spawnID(n); ok {
				in[k], changed = id, true
			}
		}
	}
	return changed
}

// messageDefaults fills the fields the harness adds to a SendMessage call: it is a message, to the
// agent it names, with the words as content.
func messageDefaults(in map[string]any) (filled bool) {
	for k, v := range map[string]any{"type": "message", "recipient": in["to"], "content": in["message"]} {
		if _, ok := in[k]; !ok && v != nil {
			in[k], filled = v, true
		}
	}
	return filled
}

// hookInput is the input of a call as its hooks are told it. A SendMessage's PreToolUse hook sees the
// call as filled in with a summary of the message, its PostToolUse hook only to, message and summary
// (recorded: runs/fgsub-maxturns).
func hookInput(before bool, tool string, input json.RawMessage) json.RawMessage {
	if tool != "SendMessage" {
		return input
	}
	var in map[string]any
	if json.Unmarshal(input, &in) != nil {
		return input
	}
	if _, ok := in["summary"]; !ok && in["message"] != nil {
		in["summary"] = in["message"]
	}
	if !before {
		in = map[string]any{"to": in["to"], "message": in["message"], "summary": in["summary"]}
	}
	b, err := marshalRecord(in)
	if err != nil {
		return input
	}
	return b
}
