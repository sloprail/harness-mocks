package replay

import (
	"fmt"
	"slices"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// lookupOf is what a GetDynamicTools block asks for when it names one tool of
// one server (namespace and toolName), which the mock does on its own before it
// calls an MCP tool; nil for any other block, GetDynamicTools searching by a
// pattern included, which the mock has no such tool for.
func lookupOf(block map[string]any) map[string]any {
	if name, _ := block["name"].(string); name != "GetDynamicTools" {
		return nil
	}
	input, _ := block["input"].(map[string]any)
	if input["namespace"] == nil || input["toolName"] == nil {
		return nil
	}
	return input
}

// sameTool reports whether the lookup named the tool the call is of.
func sameTool(lookup, call map[string]any) bool {
	return lookup != nil && lookup["namespace"] == call["server"] && lookup["toolName"] == call["tool"]
}

// placeThoughts gives each thought the index of the response, among all the
// transcript's, that had it. A request is the responses between two user
// records (the first user record is the prompt), and the requests a conversation
// thought in are taken in the order of the requests themselves. A response is
// numbered within its request by the model call it was, and a compaction is a model
// call of its own (the summary), so the responses after one are numbered one higher
// (recorded: runs/compaction-transcript-continuity): a compaction marker takes a number.
func placeThoughts(records []map[string]any, heard []thoughtAt) (map[int]*core.Thinking, error) {
	var starts []int    // the first response of each request
	var numbers [][]int // the model call number of each response of each request
	responses, skipped, fresh := 0, 0, true
	for _, rec := range records {
		switch rec["role"] {
		case "user":
			fresh = true
		case compactionMarker:
			skipped++
		case "assistant":
			if fresh {
				starts, numbers, fresh, skipped = append(starts, responses), append(numbers, nil), false, 0
			}
			numbers[len(numbers)-1] = append(numbers[len(numbers)-1], len(numbers[len(numbers)-1])+skipped)
			responses++
		}
	}
	out := map[int]*core.Thinking{}
	if responses > 0 && len(heard) == responses { // every response thought: the i-th thought is the i-th response's, whatever the model calls numbered (a compaction takes some)
		for i, h := range heard {
			out[i] = h.thinking
		}
		return out, nil
	}
	maxRequest := -1
	for _, h := range heard {
		maxRequest = max(maxRequest, h.request)
	}
	if maxRequest >= len(starts) {
		return nil, fmt.Errorf("a thought names request %d of a conversation of %d", maxRequest, len(starts))
	}
	if maxRequest >= 0 && len(starts) > 1 && maxRequest+1 != len(starts) {
		return nil, fmt.Errorf("the model thought in %d of a conversation's %d requests: which request a thought belongs to is not told", maxRequest+1, len(starts))
	}
	for _, h := range heard {
		j := slices.Index(numbers[h.request], h.response)
		if j < 0 {
			return nil, fmt.Errorf("a thought names response %d of a request whose responses are %v", h.response, numbers[h.request])
		}
		out[starts[h.request]+j] = h.thinking
	}
	return out, nil
}
