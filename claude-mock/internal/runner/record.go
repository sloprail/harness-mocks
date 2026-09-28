package runner

import (
	"encoding/json"
	"fmt"
	"strings"
)

// cliRecord is the top-level shape of a Claude Code JSONL stream-json record.
// We parse only what we need for hook-triggering, control dispatch, and validation.
//
// sr:docs https://docs.anthropic.com/en/docs/claude-code/sdk#stream-json-output-format
type cliRecord struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype,omitempty"`
	IsError bool   `json:"is_error,omitempty"`

	// IsCompactSummary marks a compaction record: real Claude Code writes a
	// {"type":"user","isCompactSummary":true,…} line to the transcript when it
	// auto-compacts the context window. A scenario script emits this to drive a
	// compaction event; the runner reacts by firing SessionStart source="compact".
	// sr:docs https://code.claude.com/docs/en/hooks#sessionstart
	IsCompactSummary bool `json:"isCompactSummary,omitempty"`

	// result frame fields
	Result string   `json:"result,omitempty"`
	Errors []string `json:"errors,omitempty"`

	// assistant frame: message.stop_reason used to detect end_turn.
	// Content is left as raw JSON: Claude Code emits it as an array of blocks for
	// normal messages but as a plain string for some records (notably the
	// isCompactSummary compaction record), and validateRecord must accept both.
	// The per-block extraction helpers (extractFirstToolUse*, hasIDLessToolResult)
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

	//   {"type":"compact"[,"logical_parent":"<uuid>"|"unwritten"][,"summary":"…"]}
	// LogicalParent overrides the boundary's logicalParentUuid — by default the
	// last record written. Real preserved-segment compactions have named a
	// logical parent that was never written to any transcript; this is how a
	// scenario reproduces that.
	LogicalParent string `json:"logical_parent,omitempty"`
	Summary       string `json:"summary,omitempty"`
	// ID, on a compact control record, is carried onto the summary record it
	// writes — so a scenario that marks each turn by an id it can find in the
	// transcript afterwards (the sloprail harness does) can see this one fired.
	ID string `json:"id,omitempty"`
	// Trigger ("auto" | "manual", default auto), Preserve (how many of the
	// last records the compaction keeps, default defaultPreserved) and
	// PreTokens (compactMetadata.preTokens; the mock spends no tokens, so 0
	// unless the scenario says) shape the compaction.
	Trigger   string `json:"trigger,omitempty"`
	Preserve  *int   `json:"preserve,omitempty"`
	PreTokens int    `json:"pre_tokens,omitempty"`
	// PostTokens is compactMetadata.postTokens (0 unless the scenario says).
	PostTokens int `json:"post_tokens,omitempty"`
	// PreservedSegment false writes the second shape real compactions left:
	// no preservedSegment/preservedMessages, and the last written record as
	// logical parent (F:compact-nohooks; 1 of 65 real boundaries).
	PreservedSegment *bool `json:"preserved_segment,omitempty"`
	// TailOffset ends the preserved segment that many records before the
	// boundary, so the logical parent (the segment's tail) is an earlier
	// written record: 7 real mid-file boundaries, 2 to 253 records back.
	TailOffset int `json:"tail_offset,omitempty"`
}

// knownTypes lists all valid JSONL record types emitted by Claude Code stream-json
// plus the mock-only control records.
// sr:docs https://docs.anthropic.com/en/docs/claude-code/sdk#stream-json-output-format
var knownTypes = map[string]bool{
	"system":    true,
	"assistant": true,
	"user":      true,
	"result":    true,
	// mock-only control records — not forwarded to stdout
	"worktree_create": true,
	"worktree_remove": true,
	"subagent_start":  true,
	"compact":         true,
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
	// The scenario script speaks for the ASSISTANT (Claude): it emits assistant
	// turns (text / tool_use), result frames, and mock control records. When it
	// emits a tool_use for a tool the mock executes (Bash/Read/Write/Edit/Glob),
	// the mock runs the tool and SYNTHESISES the tool_result itself — a scenario
	// must NOT hand-write that result, or the transcript would carry a fabricated
	// tool output the mock never produced.
	//
	// BUT a tool_result IS, in real Claude Code, delivered as a user-role message
	// — that is how EVERY tool result re-enters the conversation, and it is the
	// only shape available for tools the mock does not execute locally. The
	// motivating case is an AskUserQuestion answer envelope: the human's answer is
	// authored as a {"type":"user","message":{"content":[{"type":"tool_result",
	// "tool_use_id":…}]}} record, not synthesised from any local tool run. So a
	// scenario-authored user+tool_result is a REAL CC shape and must be accepted;
	// it is forwarded and persisted as a genuine trajectory record (scanLines does
	// not re-execute a tool for it — it only fires PostToolUse, matching the
	// "inline tool_result" path). Empirically, real CC always stamps a
	// `tool_use_id` on a tool_result; a tool_result WITHOUT one is malformed and
	// is the actual footgun the old blanket rejection was guarding against, so
	// that narrow case stays an error.
	// sr:docs https://code.claude.com/docs/en/sdk#stream-json-output-format
	if strings.EqualFold(rec.Type, "user") && hasIDLessToolResult(line) {
		return nil, fmt.Errorf("scenario emitted a tool_result block with no tool_use_id — a real Claude Code tool_result always references the tool_use it answers; add a \"tool_use_id\", or (for a tool the mock executes) emit only the tool_use and let the mock synthesise the result")
	}
	return &rec, nil
}

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
