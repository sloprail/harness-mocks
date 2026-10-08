package runner

import (
	"encoding/json"

	"github.com/sloprail/harness-mocks/internal/tools"
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
	if takeGate(cfg, m, blocks) {
		filled = true
	}
	for _, b := range blocks {
		block, _ := b.(map[string]any)
		in, _ := block["input"].(map[string]any)
		name, _ := block["name"].(string)
		if block["type"] != "tool_use" || in == nil {
			continue
		}
		if tools.IsMCPName(name) {
			filled = true // the stream's frame names the server and titles the tool (mcpToolMeta)
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
	if meta := mcpToolMeta(blocks); meta != nil {
		m["tool_use_meta"] = meta
	}
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

// takeGate removes the scenario's own gate from an assistant line and registers it: {"mock_gate":
// {"receipt_after_end":true}} says that the background command of the line's call has ended before
// its receipt is written, as the recording's event order shows it (runs/midturn); the receipt then waits
// for the command's end, whenever that is. It reports whether the line had one.
func takeGate(cfg Config, m map[string]any, blocks []any) bool {
	gate, ok := m["mock_gate"].(map[string]any)
	if !ok {
		return false
	}
	delete(m, "mock_gate")
	for _, b := range blocks {
		if block, _ := b.(map[string]any); block["type"] == "tool_use" && gate["receipt_after_end"] == true && cfg.bg != nil {
			cfg.bg.receiptAfterEnd.Store(block["id"], true)
			break
		}
	}
	return true
}
