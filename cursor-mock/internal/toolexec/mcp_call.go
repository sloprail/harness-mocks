package toolexec

import (
	"context"
	"encoding/json"
	"fmt"
)

// MCPDescribe is what the agent reads of an MCP tool before it calls it: the
// tool's name, description and input schema, as indented JSON.
func MCPDescribe(ctx context.Context, dir, server, tool string) (string, error) {
	c, err := startMCP(ctx, dir, server)
	if err != nil {
		return "", err
	}
	defer c.close()
	res, err := c.call("tools/list", map[string]any{})
	if err != nil {
		return "", err
	}
	var list struct {
		Tools []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			Schema      json.RawMessage `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(res, &list); err != nil {
		return "", err
	}
	for _, t := range list.Tools {
		if t.Name == tool {
			b, _ := json.MarshalIndent(struct {
				Tool        string          `json:"tool"`
				Description string          `json:"description"`
				Schema      json.RawMessage `json:"inputSchema"`
			}{t.Name, t.Description, t.Schema}, "", "  ")
			return string(b), nil
		}
	}
	return "", fmt.Errorf("cursor-mock: MCP server %q has no tool %q", server, tool)
}

// mcp runs an MCP tool call: the server's text content is the result. A call
// the server answers as an error, or cannot be made, is not recorded: it
// fails rather than guess.
//
// sr:provides hook-matcher-filter/cursor
func mcp(ctx context.Context, c Call, dir string) Result {
	server, tool := c.str("providerIdentifier"), c.str("toolName")
	cl, err := startMCP(ctx, dir, server)
	if err != nil {
		return failed(err.Error(), err.Error())
	}
	defer cl.close()
	res, err := cl.call("tools/call", map[string]any{"name": tool, "arguments": c.Args["args"]})
	if err != nil {
		return failed(err.Error(), err.Error())
	}
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(res, &out); err != nil || out.IsError {
		msg := "cursor-mock: an MCP tool answering with an error is not modeled"
		return failed(msg, msg)
	}
	type part struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	var frame []any
	var wire []part
	for _, p := range out.Content {
		if p.Type != "text" {
			msg := "cursor-mock: MCP content that is not text is not modeled"
			return failed(msg, msg)
		}
		frame = append(frame, map[string]any{"text": map[string]any{"text": p.Text}})
		wire = append(wire, part{"text", p.Text})
	}
	return Result{
		Frame: map[string]any{"success": map[string]any{"content": frame, "isError": false, "systemReminders": []any{}}},
		ToolOutput: jsonString(struct {
			Content []part `json:"content"`
			IsError bool   `json:"isError"`
		}{wire, false}),
	}
}
