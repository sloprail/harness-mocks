package replay

import (
	"sort"
	"strings"
)

// The model's words may name an id the harness made up for the run (a worktree is agent-<id>, and the
// model reads its path out): in a replay the words are the recording's, naming the recording's id, where the
// mock's run has an id of its own. The k-th agent the recording started is the k-th the mock did, so the
// recorded id in the mock's words stands for the mock's.

// agentIDs are the agent ids of the frames and payloads, in order of first appearance.
func agentIDs(sets ...[]map[string]any) []string {
	var ids []string
	seen := map[string]bool{}
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if id, ok := x["agent_id"].(string); ok && id != "" && !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
			keys := make([]string, 0, len(x))
			for k := range x {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				walk(x[k])
			}
		case []any:
			for _, e := range x {
				walk(e)
			}
		}
	}
	for _, set := range sets {
		for _, o := range set {
			walk(o)
		}
	}
	return ids
}

// withRecordedAgentIDs is objs with the recording's agent ids, wherever they are in the strings, replaced
// by the mock's own, by order.
func withRecordedAgentIDs(objs []map[string]any, recorded, mock []string) []map[string]any {
	if len(recorded) != len(mock) {
		return objs
	}
	pairs := make([]string, 0, 2*len(recorded))
	for i := range recorded {
		pairs = append(pairs, recorded[i], mock[i])
	}
	r := strings.NewReplacer(pairs...)
	var fix func(v any) any
	fix = func(v any) any {
		switch x := v.(type) {
		case map[string]any:
			out := make(map[string]any, len(x))
			for k, e := range x {
				out[k] = fix(e)
			}
			return out
		case []any:
			out := make([]any, len(x))
			for i, e := range x {
				out[i] = fix(e)
			}
			return out
		case string:
			return r.Replace(x)
		}
		return v
	}
	out := make([]map[string]any, len(objs))
	for i, o := range objs {
		out[i] = fix(o).(map[string]any)
	}
	return out
}
