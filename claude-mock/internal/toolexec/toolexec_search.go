package toolexec

import "encoding/json"

// executeToolSearch answers a search of the deferred tools: the mock has none, so nothing matches, and the
// structured result says how many there are and what was asked (recorded: runs/fgsub-tool-stats).
func executeToolSearch(raw json.RawMessage) Result {
	var inp struct {
		Query string `json:"query"`
	}
	_ = json.Unmarshal(raw, &inp)
	return Result{
		Output:        "No matching deferred tools found",
		ToolUseResult: map[string]any{"matches": []any{}, "query": inp.Query, "total_deferred_tools": 26},
	}
}
