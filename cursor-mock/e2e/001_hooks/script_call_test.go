package e2e

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// scriptCall is the line the scenario script prints for a tool call the
// recorded agent made: the scenario protocol's assistant line with one tool_use
// block (the Claude Code names, which the mock maps onto Cursor's tools). A
// write's content is what the recorded hooks saw written (the started frame's
// streamContent is only what the model had streamed so far); writes are the
// recorded contents not yet used, and what is left after this call is returned.
func scriptCall(t *testing.T, frame string, writes []string) (string, []string) {
	t.Helper()
	var f map[string]any
	require.NoError(t, json.Unmarshal([]byte(frame), &f))
	id, _ := f["call_id"].(string)
	var name string
	var input map[string]any
	for kind, v := range f["tool_call"].(map[string]any) {
		body, ok := v.(map[string]any)
		if !ok || !strings.HasSuffix(kind, "ToolCall") {
			continue
		}
		args := body["args"].(map[string]any)
		switch kind {
		case "shellToolCall":
			name, input = "Bash", map[string]any{"command": args["command"]}
		case "readToolCall":
			name, input = "Read", map[string]any{"file_path": args["path"]}
		case "editToolCall":
			content, _ := args["streamContent"].(string)
			if len(writes) > 0 {
				content, writes = writes[0], writes[1:]
			}
			name, input = "Write", map[string]any{"file_path": args["path"], "content": content}
		case "mcpToolCall":
			// an MCP tool the recorded agent called: the mock calls the server's
			// tool of the project's .cursor/mcp.json
			name, input = "mcp__"+args["serverIdentifier"].(string)+"__"+args["toolName"].(string), args["args"].(map[string]any)
		case "grepToolCall":
			name, input = "Grep", map[string]any{"pattern": args["pattern"]}
		case "deleteToolCall":
			name, input = "Delete", map[string]any{"file_path": args["path"]}
		case "taskToolCall":
			// a sub-agent the recorded agent started: the mock's sub-agent plays
			// the script of its Task call's input (a knob of its own), here one that
			// only replies
			name, input = "Task", map[string]any{"description": args["description"], "prompt": args["prompt"], "subagent_type": "generalPurpose", "script": "<SUBSCRIPT>"}
		}
	}
	require.NotEmpty(t, name, "a started frame naming no tool the mock runs: %s", frame)
	return jsonString(map[string]any{"type": "assistant", "message": map[string]any{"role": "assistant", "content": []any{
		map[string]any{"type": "tool_use", "id": id, "name": name, "input": input}}}}), writes
}
