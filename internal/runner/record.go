package runner

import (
	"encoding/json"
	"fmt"
	"strings"
)

// cliRecord is the top-level shape of a Claude Code JSONL stream-json record.
// We parse only what we need for hook-triggering, control dispatch, and validation.
//
// a10n:docs https://docs.anthropic.com/en/docs/claude-code/sdk#stream-json-output-format
type cliRecord struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype,omitempty"`
	IsError bool   `json:"is_error,omitempty"`

	// IsCompactSummary marks a compaction record: real Claude Code writes a
	// {"type":"user","isCompactSummary":true,…} line to the transcript when it
	// auto-compacts the context window. A scenario script emits this to drive a
	// compaction event; the runner reacts by firing SessionStart source="compact".
	// a10n:docs https://code.claude.com/docs/en/hooks#sessionstart
	IsCompactSummary bool `json:"isCompactSummary,omitempty"`

	// result frame fields
	Result string   `json:"result,omitempty"`
	Errors []string `json:"errors,omitempty"`

	// assistant frame: message.stop_reason used to detect end_turn.
	// Content is left as raw JSON: Claude Code emits it as an array of blocks for
	// normal messages but as a plain string for some records (notably the
	// isCompactSummary compaction record), and validateRecord must accept both.
	// The per-block extraction helpers (extractFirstToolUse*, lineHasToolResult)
	// re-parse the line with their own typed shapes, so this field is never decoded
	// structurally here.
	Message *struct {
		StopReason string          `json:"stop_reason,omitempty"`
		Content    json.RawMessage `json:"content"`
	} `json:"message,omitempty"`

	// mock-only control record fields (never forwarded to stdout):
	//   {"type":"worktree_create","worktree_name":"feat/foo"}
	//   {"type":"worktree_remove","worktree_name":"feat/foo"}
	//   {"type":"subagent_start","agent_type":"claude"}
	WorktreeName string `json:"worktree_name,omitempty"`
	AgentType    string `json:"agent_type,omitempty"`
}

// knownTypes lists all valid JSONL record types emitted by Claude Code stream-json
// plus the mock-only control records.
// a10n:docs https://docs.anthropic.com/en/docs/claude-code/sdk#stream-json-output-format
var knownTypes = map[string]bool{
	"system":    true,
	"assistant": true,
	"user":      true,
	"result":    true,
	// mock-only control records — not forwarded to stdout
	"worktree_create": true,
	"worktree_remove": true,
	"subagent_start":  true,
}

// validateRecord ensures the JSONL line is parseable JSON with a non-empty "type"
// field that belongs to the known set.
func validateRecord(line []byte) (*cliRecord, error) {
	var rec cliRecord
	if err := json.Unmarshal(line, &rec); err != nil {
		return nil, fmt.Errorf("not valid JSON: %w", err)
	}
	if rec.Type == "" {
		return nil, fmt.Errorf("missing required field \"type\"")
	}
	if !knownTypes[strings.ToLower(rec.Type)] {
		return nil, fmt.Errorf("unknown record type %q (expected: system, assistant, user, result)", rec.Type)
	}
	// The scenario script speaks for the ASSISTANT (Claude): it may emit
	// assistant turns (text / tool_use), result frames, and mock control records.
	// tool_result blocks are the MOCK's job — they are synthesised after the mock
	// executes a tool. A scenario that emits its own tool_result is modelling
	// something Claude Code never produces, so reject it loudly rather than
	// letting a bogus transcript through.
	if strings.EqualFold(rec.Type, "user") && lineHasToolResult(line) {
		return nil, fmt.Errorf("scenario emitted a tool_result block — that is synthesised by the mock after it executes a tool, not by the agent script; emit the tool_use and let the mock produce the result")
	}
	return &rec, nil
}

// lineHasToolResult reports whether a user record carries a tool_result content block.
func lineHasToolResult(line []byte) bool {
	var rec struct {
		Message *struct {
			Content []struct {
				Type string `json:"type"`
			} `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal(line, &rec) != nil || rec.Message == nil {
		return false
	}
	for _, c := range rec.Message.Content {
		if c.Type == "tool_result" {
			return true
		}
	}
	return false
}

// extractFirstToolUseWithID finds the first tool_use content block in an assistant line.
// Returns ("", "", nil) if there is none.
//
// a10n:docs https://docs.anthropic.com/en/docs/claude-code/sdk#stream-json-output-format
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

// extractFirstToolResult finds the first tool_result content block in a user line.
// Returns ("", nil) if there is none.
func extractFirstToolResult(line []byte) (toolName string, toolOutput json.RawMessage) {
	var rec struct {
		Message *struct {
			Content []struct {
				Type    string          `json:"type"`
				Name    string          `json:"name"`
				Content json.RawMessage `json:"content"`
			} `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(line, &rec); err != nil || rec.Message == nil {
		return "", nil
	}
	for _, block := range rec.Message.Content {
		if block.Type == "tool_result" && block.Name != "" {
			return block.Name, block.Content
		}
	}
	return "", nil
}
