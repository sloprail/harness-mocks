package replay

import (
	"regexp"
	"time"
)

// stampOf is when a transcript record was written, zero when it carries no time.
func stampOf(rec map[string]any) time.Time {
	s, _ := rec["timestamp"].(string)
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// resultIDs are the tool_use ids a user record gives results for.
func resultIDs(rec map[string]any) map[string]bool {
	out := map[string]bool{}
	msg, _ := rec["message"].(map[string]any)
	blocks, _ := msg["content"].([]any)
	for _, b := range blocks {
		block, _ := b.(map[string]any)
		if block["type"] == "tool_result" {
			if id, _ := block["tool_use_id"].(string); id != "" {
				out[id] = true
			}
		}
	}
	return out
}

// withoutForkContext is a sub-agent's records without the context a fork inherits: its transcript starts
// with a fork-context-ref record, then the parent's call that started it and the result that answered
// that call (recorded: runs/nested-fork-limit). What follows is what the fork itself did.
func withoutForkContext(records []map[string]any) []map[string]any {
	if len(records) == 0 || records[0]["type"] != "fork-context-ref" {
		return records
	}
	started := ""
	for i, rec := range records[1:] {
		if started == "" && rec["type"] == "assistant" {
			msg, _ := rec["message"].(map[string]any)
			blocks, _ := msg["content"].([]any)
			for _, b := range blocks {
				if block, _ := b.(map[string]any); block["type"] == "tool_use" {
					started, _ = block["id"].(string)
				}
			}
		}
		if started != "" && rec["type"] == "user" && resultIDs(rec)[started] {
			return records[i+2:]
		}
	}
	return records
}

var agentIDLine = regexp.MustCompile(`agentId: (\w+)`)

// agentIDs are the ids of the sub-agents the calls started, by the id of the call: a call's result names
// its agent in a line "agentId: <id>".
func agentIDs(records []map[string]any) map[string]string {
	out := map[string]string{}
	for _, rec := range records {
		msg, _ := rec["message"].(map[string]any)
		blocks, _ := msg["content"].([]any)
		for _, b := range blocks {
			block, _ := b.(map[string]any)
			if block["type"] != "tool_result" {
				continue
			}
			id, _ := block["tool_use_id"].(string)
			var text string
			switch c := block["content"].(type) {
			case string:
				text = c
			case []any:
				for _, p := range c {
					part, _ := p.(map[string]any)
					t, _ := part["text"].(string)
					text += t
				}
			}
			if m := agentIDLine.FindStringSubmatch(text); m != nil && id != "" {
				out[id] = m[1]
			}
		}
	}
	return out
}
