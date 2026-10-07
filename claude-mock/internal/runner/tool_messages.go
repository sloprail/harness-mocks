package runner

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/sloprail/harness-mocks/internal/tools"
)

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
	if tools.IsMCPName(tool) { // the arguments the server's tool takes: the mock's own mock_result is not among them
		var in map[string]json.RawMessage
		if json.Unmarshal(input, &in) != nil || in["mock_result"] == nil {
			return input
		}
		delete(in, "mock_result")
		if b, err := marshalRecord(in); err == nil {
			return b
		}
		return input
	}
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

// mcpToolMeta is the tool_use_meta of the main agent's assistant frame for the MCP tools it calls: each
// call's id, the tool titled and the server named (recorded: snapshots/runs/mcp-tool); nil with none.
// sr:docs https://code.claude.com/docs/en/mcp
func mcpToolMeta(blocks []any) []any {
	var meta []any
	for _, b := range blocks {
		block, _ := b.(map[string]any)
		name, _ := block["name"].(string)
		if server, tool, ok := tools.MCPName(name); ok && block["type"] == "tool_use" {
			meta = append(meta, map[string]any{"id": block["id"], "display_name": tools.MCPDisplayName(tool), "server_display_name": server})
		}
	}
	return meta
}
