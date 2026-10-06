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
