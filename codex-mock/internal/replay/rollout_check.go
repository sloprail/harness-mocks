package replay

import (
	"fmt"
	"strings"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// checkOutput holds what a script's recorded output says against the calls read
// out of it: a script that failed stopped at a call that threw (a call a hook
// refused, say), and which of several calls that was is not in the recording, so
// a failed script may have made one call only.
func checkOutput(output map[string]any, derived map[any]int) error {
	n, known := derived[output["call_id"]]
	if !known {
		return fmt.Errorf("a tool output for a call that no script made")
	}
	if n > 1 && strings.Contains(outputText(output["output"]), "Script failed") {
		return fmt.Errorf("a script that made %d calls failed: which of them ran is not recorded", n)
	}
	return nil
}

// outputText is the text of a tool output: a string, or a list of text items.
func outputText(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	var b strings.Builder
	items, _ := v.([]any)
	for _, it := range items {
		m, _ := it.(map[string]any)
		s, _ := m["text"].(string)
		b.WriteString(s)
	}
	return b.String()
}

// attachReceipts gives each spawn_agent call that was answered with a receipt the
// id the run's stream names for it (receipts, in order, none for a spawn that
// was refused): the key of the sub-agent's recorded rollout. Nil receipts is a
// rollout whose stream is not the run's (a sub-agent's): no call is given one.
func attachReceipts(calls []core.Call, told []string) error {
	if told == nil {
		return nil
	}
	next := 0
	for i := range calls {
		if calls[i].Tool != core.ToolSpawn || calls[i].Input["message"] == nil {
			continue
		}
		if next >= len(told) {
			return fmt.Errorf("a spawn_agent that the stream shows no sub-agent started for")
		}
		calls[i].Ref = told[next]
		next++
	}
	return nil
}

// spawnReceipts are the sub-agents the run's stream shows the main thread
// started, in order: the receiver of each completed spawn_agent item.
func spawnReceipts(stream []map[string]any, main string) []string {
	out := []string{}
	for _, l := range stream {
		item, _ := l["item"].(map[string]any)
		if l["type"] != "item.completed" || item["type"] != "collab_tool_call" || item["tool"] != "spawn_agent" || item["sender_thread_id"] != main {
			continue
		}
		if ids, _ := item["receiver_thread_ids"].([]any); len(ids) == 1 {
			if id, ok := ids[0].(string); ok {
				out = append(out, id)
			}
		}
	}
	return out
}
