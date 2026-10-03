package runner

import (
	"bytes"
	"encoding/json"
	"os"

	"github.com/sloprail/harness-mocks/internal/subagents"
)

// toolClasses are Claude Code's tools by how a sub-agent's result counts them,
// as recorded (snapshots/runs/fgsub-tool-stats: Write and Edit are edits,
// ToolSearch other; hookmix and fg-subagent-bash: Read and Bash). Grep and Glob
// are searches by the field's name: no recording has a sub-agent call them.
var toolClasses = map[string]subagents.Class{
	"Read": subagents.Read, "Grep": subagents.Search, "Glob": subagents.Search, "Bash": subagents.Shell,
	"Write": subagents.Edit, "Edit": subagents.Edit, toolNameAgent: subagents.Skipped, toolNameTask: subagents.Skipped,
}

// toolStatsOf is the toolStats a finished foreground sub-agent's result
// carries, tallied from the tool calls in its own transcript. A sub-agent that
// made no counted call has none: Agent calls are not counted (snapshots/runs/meta,
// an outer sub-agent that only dispatched one).
//
// sr:provides foreground-subagent-result/claude
func toolStatsOf(sidechain string) map[string]any {
	raw, err := os.ReadFile(sidechain)
	if err != nil {
		return nil
	}
	var calls []subagents.Call
	for _, line := range bytes.Split(raw, []byte{'\n'}) {
		var rec struct {
			Type    string `json:"type"`
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &rec) != nil || rec.Type != "assistant" {
			continue
		}
		var blocks []struct {
			Type  string `json:"type"`
			Name  string `json:"name"`
			Input struct {
				Content   string `json:"content"`
				OldString string `json:"old_string"`
				NewString string `json:"new_string"`
			} `json:"input"`
		}
		if json.Unmarshal(rec.Message.Content, &blocks) != nil {
			continue
		}
		for _, b := range blocks {
			if b.Type == "tool_use" {
				// a Write adds its content; an Edit adds its new text and takes its old
				calls = append(calls, subagents.Call{Tool: b.Name, Added: b.Input.Content + b.Input.NewString, Taken: b.Input.OldString})
			}
		}
	}
	c := subagents.Tally(calls, toolClasses)
	if c.Total() == 0 {
		return nil
	}
	return map[string]any{
		"readCount": c.Read, "searchCount": c.Search, "bashCount": c.Shell, "editFileCount": c.Edits,
		"linesAdded": c.LinesAdded, "linesRemoved": c.LinesRemoved, "otherToolCount": c.Other,
	}
}

// zeroUsage is the usage object a sub-agent's result carries, with the mock's
// token counts: it spends none.
func zeroUsage() map[string]any {
	return map[string]any{
		"output_tokens_details": map[string]any{"thinking_tokens": 0},
		"input_tokens":          0, "cache_creation_input_tokens": 0, "cache_read_input_tokens": 0, "output_tokens": 0,
		"server_tool_use": map[string]any{"web_search_requests": 0, "web_fetch_requests": 0},
		"service_tier":    "standard",
		"cache_creation":  map[string]any{"ephemeral_1h_input_tokens": 0, "ephemeral_5m_input_tokens": 0},
		"inference_geo":   "not_available", "iterations": []any{}, "speed": "standard", "fallback_credit": nil,
	}
}

// textBlocks is a report as the content of a result: one text block, none for
// an empty report.
func textBlocks(report string) []map[string]any {
	if report == "" {
		return []map[string]any{}
	}
	return []map[string]any{{"type": "text", "text": report}}
}
