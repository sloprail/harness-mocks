package replay

import (
	core "github.com/sloprail/harness-mocks/internal/replay"
)

// The unified names of the file tools, which only the claude adapter maps.
const (
	toolRead  = "read"
	toolWrite = "write"
	toolEdit  = "edit"
	toolGlob  = "glob"
)

// fileTools are the file tools' unified names by claude's.
var fileTools = map[string]string{"Read": toolRead, "Write": toolWrite, "Edit": toolEdit, "Glob": toolGlob}

// the stream's assistant frames hold them (wire_tool_inputs): the transcript has them as the harness
// went on to fill in what the model left out.
func wireInputs(stream []map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, f := range stream {
		wire, _ := f["wire_tool_inputs"].(map[string]any)
		for id, in := range wire {
			if m, ok := in.(map[string]any); ok {
				out[id] = m
			}
		}
	}
	return out
}

// withWireInputs is the turns with each call's input as the model sent it, where the stream says: the
// mock fills in what the harness filled in, and the stream names both.
func withWireInputs(t turns, wire map[string]map[string]any) turns {
	calls := append([]core.Call(nil), t.agent.Calls...)
	for i, c := range calls {
		if in, ok := wire[t.ids[i]]; ok && c.Tool != core.ToolSpawn {
			c.Input = in
			calls[i] = c
		}
	}
	t.agent.Calls = calls
	return t
}

// gateKey marks, among a call's input as the adapter carries it, that the receipt of its background command
// came after the command's end (mockCall moves it to the script line's mock_gate).
const gateKey = "mock_gate_receipt_after_end"

// receiptsAfterEnd are the ids of the calls whose task had ended, by the stream's order, before the
// result that answers the call: a background command that was done when its receipt was written.
func receiptsAfterEnd(stream []map[string]any) map[string]bool {
	ended, out := map[string]bool{}, map[string]bool{}
	for _, f := range stream {
		if id, _ := f["tool_use_id"].(string); f["subtype"] == "task_notification" && id != "" {
			ended[id] = true
		}
		msg, _ := f["message"].(map[string]any)
		blocks, _ := msg["content"].([]any)
		for _, b := range blocks {
			if block, _ := b.(map[string]any); block["type"] == "tool_result" {
				id, _ := block["tool_use_id"].(string)
				if ended[id] {
					out[id] = true
				}
			}
		}
	}
	return out
}

// withReceiptGates is the turns with the calls that gate their receipt on their task's end marked.
func withReceiptGates(t turns, gated map[string]bool) turns {
	calls := append([]core.Call(nil), t.agent.Calls...)
	for i, c := range calls {
		if gated[t.ids[i]] && c.Tool == core.ToolShell {
			in := make(map[string]any, len(c.Input)+1)
			for k, v := range c.Input {
				in[k] = v
			}
			in[gateKey] = true
			c.Input = in
			calls[i] = c
		}
	}
	t.agent.Calls = calls
	return t
}

// framedBash are the ids of the Bash calls whose run left task frames in the stream: the harness
// streams them for a call that ran long, or one a background agent made.
func framedBash(stream []map[string]any) map[string]bool {
	out := map[string]bool{}
	for _, f := range stream {
		if f["type"] == "system" && f["subtype"] == "task_started" && f["task_type"] == "local_bash" {
			if id, _ := f["tool_use_id"].(string); id != "" {
				out[id] = true
			}
		}
	}
	return out
}

// withTaskFrames is the turns with each foreground Bash call saying whether it left task frames
// (the mock's own task_frames key), as the recording shows.
func withTaskFrames(t turns, framed map[string]bool) turns {
	calls := append([]core.Call(nil), t.agent.Calls...)
	for i, c := range calls {
		if c.Tool != core.ToolShell || c.Input["run_in_background"] == true {
			continue
		}
		in := copyInput(c.Input)
		in["task_frames"] = framed[t.ids[i]]
		c.Input = in
		calls[i] = c
	}
	t.agent.Calls = calls
	return t
}
