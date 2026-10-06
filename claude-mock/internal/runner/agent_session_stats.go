package runner

import (
	"encoding/json"

	"github.com/sloprail/harness-mocks/internal/subagents"
)

// askOf is what an Agent call asked for with run_in_background: true, false, or
// nothing.
func askOf(input json.RawMessage) subagents.Ask {
	var in struct {
		Background *bool `json:"run_in_background"`
	}
	if json.Unmarshal(input, &in) != nil || in.Background == nil {
		return subagents.Unset
	}
	if *in.Background {
		return subagents.AskedBackground
	}
	return subagents.AskedForeground
}

// withSubagentStats is a result frame carrying the session's sub-agent tally as
// subagent_stats, the way the real claude's does (recorded: every sub-agent run's
// last result frame, snapshots/runs/meta, bgagent-concurrent-limit): sub-agents
// spawned, what their calls requested and how many started in the background,
// the deepest, those spawned by sub-agents, how many completed or failed, the
// spawns refused for the concurrent limit, and the count by type. What the mock
// has no case of stays 0: sub-agents killed, and spawns refused for budget.
//
// sr:provides background-agent/claude
func withSubagentStats(line []byte, bg *backgroundTasks) []byte {
	var frame map[string]any
	if bg == nil || json.Unmarshal(line, &frame) != nil || frame["type"] != "result" || frame["subagent_stats"] != nil || line[0] != '{' {
		return line
	}
	t := bg.stats.Count()
	byType := map[string]any{}
	for k, v := range t.ByType {
		byType[k] = v
	}
	stats, err := json.Marshal(map[string]any{
		"spawned": t.Spawned, "started_in_background": t.StartedInBackground, "max_depth": t.MaxDepth,
		"spawned_by_subagents": t.SpawnedBySubagents, "completed": t.Completed, "failed": t.Failed,
		"requested": map[string]any{"background": t.Requested[subagents.AskedBackground], "foreground": t.Requested[subagents.AskedForeground], "unset": t.Requested[subagents.Unset]},
		"killed":    map[string]any{"parent": 0, "user": 0, "system": 0},
		"refused":   map[string]any{"depth_limit": t.RefusedDepth, "concurrency_limit": t.RefusedConcurrency, "budget": 0},
		"by_type":   byType,
	})
	if err != nil {
		return line
	}
	// at the front, so the frame keeps the order of the script's own fields
	return append([]byte(`{"subagent_stats":`+string(stats)+`,`), line[1:]...)
}
