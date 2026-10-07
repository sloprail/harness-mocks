package replay

import (
	core "github.com/sloprail/harness-mocks/internal/replay"
)

// recorded is what a tool answered, as the stream shows it: the text the agent got and the structured
// result beside it.
type recorded struct {
	content    any
	structured map[string]any
	isError    bool
}

// scriptedTools are the tools whose effect the mock cannot produce (it reaches no web): a script gives
// the call the result to answer with, in its own mock_result input, and the replay gives it the recorded one.
var scriptedTools = map[string]bool{toolPrefix + "WebFetch": true, toolPrefix + "WebSearch": true}

// recordedResults are the results of the calls, by call id, from the stream's tool_result frames.
func recordedResults(stream []map[string]any) map[string]recorded {
	out := map[string]recorded{}
	for _, f := range stream {
		if f["type"] != "user" {
			continue
		}
		msg, _ := f["message"].(map[string]any)
		blocks, _ := msg["content"].([]any)
		for _, b := range blocks {
			block, _ := b.(map[string]any)
			id, _ := block["tool_use_id"].(string)
			if block["type"] != "tool_result" || id == "" {
				continue
			}
			structured, _ := f["tool_use_result"].(map[string]any)
			isError, _ := block["is_error"].(bool)
			out[id] = recorded{content: block["content"], structured: structured, isError: isError}
		}
	}
	return out
}

// withMockResults is the turns with the recorded result put in the mock_result input of each call of a
// tool the mock cannot carry out: the answer the real harness gave, which the mock returns in that shape.
func withMockResults(t turns, results map[string]recorded) turns {
	calls := append([]core.Call(nil), t.agent.Calls...)
	for i, c := range calls {
		r, ok := results[t.ids[i]]
		if !scriptedTools[c.Tool] || !ok {
			continue
		}
		in := copyInput(c.Input)
		in["mock_result"] = mockResult(c.Tool, r)
		c.Input = in
		calls[i] = c
	}
	t.agent.Calls = calls
	return t
}

// mockResult is a recorded result as the mock's mock_result input names it. A call the mock refuses
// itself (an error result) is given an empty one, which it never reads.
func mockResult(tool string, r recorded) map[string]any {
	if r.isError {
		return map[string]any{}
	}
	switch tool {
	case toolPrefix + "WebFetch":
		out := map[string]any{}
		for _, k := range []string{"result", "bytes", "code", "codeText"} {
			if v, ok := r.structured[k]; ok {
				out[k] = v
			}
		}
		return out
	case toolPrefix + "WebSearch":
		out := map[string]any{}
		if results, _ := r.structured["results"].([]any); len(results) == 2 {
			if found, _ := results[0].(map[string]any); found != nil {
				out["links"] = found["content"]
			}
			out["findings"] = results[1]
		}
		if n, ok := r.structured["searchCount"]; ok {
			out["searchCount"] = n
		}
		return out
	}
	return map[string]any{}
}
