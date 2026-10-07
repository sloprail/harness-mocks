package toolexec

import (
	"encoding/json"
	"fmt"

	"github.com/sloprail/harness-mocks/internal/tools"
)

// mcpInput is what the mock reads of an MCP tool's input: the mock's own mock_result, the result the
// script gives the call, in the shape of an MCP tool result (content blocks, and isError). The rest of
// the input is the server's tool's arguments, which the mock does not look at.
type mcpInput struct {
	MockResult *struct {
		Content []map[string]any `json:"content"`
		IsError bool             `json:"isError"`
	} `json:"mock_result"`
}

// executeMCP answers a call of an MCP server's tool the way claude 2.1.285 does (recorded:
// snapshots/runs/mcp-tool): a result is the server's content blocks, which are the tool_result's content,
// its structured result and PostToolUse's tool_response alike; a result the server marks isError is
// an error that ran and failed, its text alone what the agent gets (PostToolUseFailure). The mock starts
// no server: a call the script gave no result is an error result saying so.
//
// sr:provides mcp-tool/claude
func executeMCP(name string, raw json.RawMessage) Result {
	var inp mcpInput
	if err := json.Unmarshal(raw, &inp); err != nil {
		return Result{Output: fmt.Sprintf("%s: the input is not a JSON object", name), IsError: true}
	}
	if inp.MockResult == nil {
		return Result{Output: fmt.Sprintf("%s: the mock runs no MCP server: give the call a mock_result with the result to return", name), IsError: true}
	}
	r := inp.MockResult
	if r.IsError {
		return failed(tools.MCPText(r.Content))
	}
	blocks := r.Content
	if blocks == nil {
		blocks = []map[string]any{}
	}
	return Result{Output: tools.MCPText(blocks), Blocks: blocks, ToolUseResult: blocks}
}
