package replay

import (
	"path/filepath"
	"sort"
)

// rolloutCalls are the tool calls the rollouts in paths record, as objects to compare: for each
// rollout (the main thread's, then each sub-agent's, in the order they began) a marker line and then
// one object per call, in the recording's names ("tool", "input"). They are read out of the JS each
// call is made from (RolloutCalls), so what is compared is the call, not how the JS is spelled.
func rolloutCalls(paths []string) []map[string]any {
	type rollout struct {
		began string
		path  string
		calls []RolloutCall
	}
	var rollouts []rollout
	for _, p := range paths {
		records, err := parseJSONL(readFile(p))
		if err != nil || len(records) == 0 {
			continue
		}
		began, _ := records[0]["timestamp"].(string)
		rollouts = append(rollouts, rollout{began, filepath.Base(p), RolloutCalls(records)})
	}
	sort.SliceStable(rollouts, func(i, j int) bool {
		if rollouts[i].began != rollouts[j].began {
			return rollouts[i].began < rollouts[j].began
		}
		return rollouts[i].path < rollouts[j].path
	})
	var out []map[string]any
	for i, r := range rollouts {
		out = append(out, map[string]any{"rollout": i + 1})
		for _, c := range r.calls {
			out = append(out, map[string]any{"tool": c.Tool, "input": named(c)})
		}
	}
	return out
}

// recordedRollouts are the rollouts a recorded sample kept; the mock's are under its CODEX_HOME.
func recordedRollouts(sample string) []string {
	paths, _ := filepath.Glob(filepath.Join(sample, "transcript", "rollout-*.jsonl"))
	return paths
}

// named is the call's input with what differs in every run by name only, which the run's own
// stream and hooks compare by identity: the agents a wait_agent waits for (a recorded script names
// them by the receipt it was told, which the JS reads from a result, and a literal id is another
// run's) and the session of a command left running (a pid).
func named(c RolloutCall) map[string]any {
	in := map[string]any{}
	for k, v := range c.Input {
		in[k] = v
	}
	switch {
	case c.Tool == "multi_agent_v1__wait_agent":
		if ts, ok := in["targets"].([]any); ok {
			masked := make([]any, len(ts))
			for i := range ts {
				masked[i] = "<agent>"
			}
			in["targets"] = masked
		}
	case c.Tool == "write_stdin":
		if _, ok := in["session_id"]; ok {
			in["session_id"] = "<session>"
		}
	}
	return in
}
