package runner

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
)

// toolStatsOf is the toolStats a finished foreground sub-agent's result
// carries, counted from the tool calls in its own transcript, as recorded
// (snapshots/runs/fgsub-tool-stats: Write and Edit count as edits, with the
// lines they add and remove, ToolSearch as other; hookmix and fg-subagent-bash:
// Read and Bash). A sub-agent that made no such call has none: Agent calls are
// not counted (snapshots/runs/meta, an outer sub-agent that only dispatched
// one). Grep and Glob count as searches by the field's name: no recording has
// a sub-agent call them.
//
// sr:provides foreground-subagent-result/claude
func toolStatsOf(sidechain string) map[string]any {
	raw, err := os.ReadFile(sidechain)
	if err != nil {
		return nil
	}
	var read, search, bash, edit, added, removed, other int
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
			if b.Type != "tool_use" {
				continue
			}
			switch b.Name {
			case "Read":
				read++
			case "Grep", "Glob":
				search++
			case "Bash":
				bash++
			case "Write":
				edit++
				added += lineCount(b.Input.Content)
			case "Edit":
				edit++
				added += lineCount(b.Input.NewString)
				removed += lineCount(b.Input.OldString)
			case toolNameAgent, toolNameTask:
			default:
				other++
			}
		}
	}
	if read+search+bash+edit+other == 0 {
		return nil
	}
	return map[string]any{
		"readCount": read, "searchCount": search, "bashCount": bash, "editFileCount": edit,
		"linesAdded": added, "linesRemoved": removed, "otherToolCount": other,
	}
}

// lineCount is how many lines s has; none for an empty string.
func lineCount(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
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
