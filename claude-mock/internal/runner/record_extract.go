package runner

import (
	"encoding/json"
)

// hasIDLessToolResult reports whether a user record carries a tool_result block
// that lacks a tool_use_id. Such a block is malformed: real Claude Code always
// references the answered tool_use, so a scenario emitting one either fabricated
// a result for a tool the mock executes, or wrote a broken answer envelope. A
// well-formed authored tool_result (with a tool_use_id) is a real CC shape and is
// accepted.
func hasIDLessToolResult(line []byte) bool {
	var rec struct {
		Message *struct {
			Content []struct {
				Type      string `json:"type"`
				ToolUseID string `json:"tool_use_id"`
			} `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal(line, &rec) != nil || rec.Message == nil {
		return false
	}
	for _, c := range rec.Message.Content {
		if c.Type == "tool_result" && c.ToolUseID == "" {
			return true
		}
	}
	return false
}

// extractFirstToolUseWithID finds the first tool_use content block in an assistant line.
// Returns ("", "", nil) if there is none.
//
// sr:docs https://docs.anthropic.com/en/docs/claude-code/sdk#stream-json-output-format
func extractFirstToolUseWithID(line []byte) (toolUseID, toolName string, toolInput json.RawMessage) {
	var rec struct {
		Message *struct {
			Content []struct {
				Type  string          `json:"type"`
				ID    string          `json:"id"`
				Name  string          `json:"name"`
				Input json.RawMessage `json:"input"`
			} `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(line, &rec); err != nil || rec.Message == nil {
		return "", "", nil
	}
	for _, block := range rec.Message.Content {
		if block.Type == "tool_use" && block.Name != "" {
			return block.ID, block.Name, block.Input
		}
	}
	return "", "", nil
}

// extractFirstToolUse is a convenience wrapper that drops the tool_use_id.
// a10n:blueprint:ignore
func extractFirstToolUse(line []byte) (toolName string, toolInput json.RawMessage) {
	_, name, input := extractFirstToolUseWithID(line)
	return name, input
}

// extractFirstToolResult finds the first tool_result content block in a user
// line: the tool_use_id it answers, the tool name when the scenario put one on
// the block (a real tool_result names no tool — the caller then looks the id
// up), and its content. All empty if there is none.
func extractFirstToolResult(line []byte) (toolUseID, toolName string, toolOutput json.RawMessage) {
	var rec struct {
		Message *struct {
			Content []struct {
				Type      string          `json:"type"`
				ToolUseID string          `json:"tool_use_id"`
				Name      string          `json:"name"`
				Content   json.RawMessage `json:"content"`
			} `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(line, &rec); err != nil || rec.Message == nil {
		return "", "", nil
	}
	for _, block := range rec.Message.Content {
		if block.Type == "tool_result" {
			return block.ToolUseID, block.Name, block.Content
		}
	}
	return "", "", nil
}
