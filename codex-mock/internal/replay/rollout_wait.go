package replay

import (
	"encoding/json"
	"fmt"
	"slices"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// unifyWait is the unified wait of a tools.multi_agent_v1__wait_agent call: each
// target is a sub-agent the agent spawned earlier, named by the receipt the
// script holds (spawned.agent_id) or by an id literal the model was told of;
// the unified target is the position of its spawn among the agent's spawns.
func unifyWait(arg map[string]any, spawns []int, told []string) (core.Call, error) {
	list, ok := arg["targets"].([]any)
	if !ok {
		return core.Call{}, fmt.Errorf("a wait_agent whose targets are not a list")
	}
	targets := make([]int, len(list))
	for i, t := range list {
		pos := -1
		switch v := t.(type) {
		case ref:
			if v.path == ".agent_id" {
				pos = slices.Index(spawns, v.call)
			}
		case string:
			pos = slices.Index(told, v)
		}
		if pos < 0 {
			return core.Call{}, fmt.Errorf("a wait_agent for something that is not a sub-agent the model spawned")
		}
		targets[i] = pos
	}
	timeout, ok := arg["timeout_ms"].(number)
	if !ok {
		return core.Call{}, fmt.Errorf("a wait_agent whose timeout_ms is not a number")
	}
	return core.Call{Tool: core.ToolWait, Input: map[string]any{"targets": targets, "timeout_ms": int(timeout.f)}}, nil
}

// agentIDs are the sub-agent ids a tool call's output tells the model of: the
// output is a list of text items, and an item whose text is a JSON object with an
// agent_id (a spawn_agent receipt, as the script printed it) names one.
func agentIDs(output any) (ids []string) {
	items, _ := output.([]any)
	for _, it := range items {
		m, _ := it.(map[string]any)
		text, _ := m["text"].(string)
		var r struct {
			AgentID string `json:"agent_id"`
		}
		if json.Unmarshal([]byte(text), &r) == nil && r.AgentID != "" {
			ids = append(ids, r.AgentID)
		}
	}
	return ids
}
