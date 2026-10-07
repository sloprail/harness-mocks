package runner

import "encoding/json"

// mockResultKey is the input key of a tool the mock cannot run (WebFetch, WebSearch, an MCP server's tool): the
// result a script gives the call. It is the mock's own, so nothing real claude shows of a call has it: not its
// hooks, its transcript or its stream.
const mockResultKey = "mock_result"

// acceptsMockResult: whether the tool takes a mock_result, as the schema declares it: a mock-only parameter of
// the tool, or the open arguments of an MCP tool.
func acceptsMockResult(tool string) bool {
	t, ok := Schema().Tool(tool)
	if !ok {
		return false
	}
	if t.Open {
		return true
	}
	for _, p := range t.Params {
		if p.Name == mockResultKey && p.MockOnly {
			return true
		}
	}
	return false
}

// withoutMockResult is a call's input without its mock_result.
func withoutMockResult(input json.RawMessage) json.RawMessage {
	var in map[string]json.RawMessage
	if json.Unmarshal(input, &in) != nil || in[mockResultKey] == nil {
		return input
	}
	delete(in, mockResultKey)
	if b, err := marshalRecord(in); err == nil {
		return b
	}
	return input
}

// withoutMockResults is an assistant line with the mock_result of its tool calls taken out, of the calls and of
// the inputs as the model sent them (wire_tool_inputs); the call the mock runs keeps its own copy.
func withoutMockResults(line []byte) []byte {
	var m map[string]any
	if json.Unmarshal(line, &m) != nil || m["type"] != "assistant" {
		return line
	}
	changed := false
	strip := func(in map[string]any) {
		if _, ok := in[mockResultKey]; ok {
			delete(in, mockResultKey)
			changed = true
		}
	}
	msg, _ := m["message"].(map[string]any)
	blocks, _ := msg["content"].([]any)
	for _, b := range blocks {
		block, _ := b.(map[string]any)
		name, _ := block["name"].(string)
		if in, ok := block["input"].(map[string]any); ok && block["type"] == "tool_use" && acceptsMockResult(name) {
			strip(in)
		}
	}
	if wire, ok := m["wire_tool_inputs"].(map[string]any); ok {
		for _, in := range wire {
			if w, ok := in.(map[string]any); ok {
				strip(w)
			}
		}
	}
	if !changed {
		return line
	}
	if out, err := marshalRecord(m); err == nil {
		return out
	}
	return line
}
