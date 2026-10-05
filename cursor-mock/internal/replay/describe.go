package replay

import core "github.com/sloprail/harness-mocks/internal/replay"

// describeMCPCalls gives each MCP call of the main agent the description the
// model wrote for it, which only the stream holds (the call's started frame
// carries it beside its args, and the transcript's block does not): the calls
// and the stream's mcpToolCall frames are in the same order.
func describeMCPCalls(a *core.Agent, stream []map[string]any) {
	var descriptions []string
	for _, f := range stream {
		if f["type"] != "tool_call" || f["subtype"] != "started" {
			continue
		}
		tc, _ := f["tool_call"].(map[string]any)
		if body, ok := tc["mcpToolCall"].(map[string]any); ok {
			d, _ := body["description"].(string)
			descriptions = append(descriptions, d)
		}
	}
	i := 0
	for n := range a.Calls {
		c := &a.Calls[n]
		if c.Tool != core.ToolMCP {
			continue
		}
		if i < len(descriptions) && descriptions[i] != "" {
			c.Input["description"] = descriptions[i]
		}
		i++
	}
}
