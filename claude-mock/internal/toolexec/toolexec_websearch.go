package toolexec

import (
	"crypto/rand"
	"encoding/json"

	"github.com/sloprail/harness-mocks/internal/tools"
)

// webSearchInput is the argument shape for the WebSearch tool: the query, the domains to keep and the
// mode of the real tool, and the mock's own mock_result, the result a script gives the call (the mock
// reaches no web): the pages found, the findings drawn from them and the number of searches made.
// sr:docs https://code.claude.com/docs/en/tools-reference#websearch-tool-behavior
type webSearchInput struct {
	Query      string `json:"query"`
	MockResult *struct {
		Links        []tools.SearchLink `json:"links"`
		Findings     string             `json:"findings"`
		SearchCount  *int               `json:"searchCount"`
		DurationSecs float64            `json:"durationSeconds"`
	} `json:"mock_result"`
}

// executeWebSearch answers a WebSearch the way claude 2.1.285 does (recorded: snapshots/runs/web-search-tool):
// the result the script gave the call, in the recorded shape: a text of the query, the pages found, the
// findings and the reminder to cite them, and a toolUseResult of the query, the pages (under the id of the
// search that found them) with the findings, the duration and the number of searches. The mock searches
// nothing: a call the script gave no result is an error result saying so.
//
// sr:provides web-search-tool/claude
func executeWebSearch(raw json.RawMessage) Result {
	var inp webSearchInput
	if err := json.Unmarshal(raw, &inp); err != nil || inp.Query == "" {
		return Result{Output: "WebSearch: missing or invalid 'query' field", IsError: true}
	}
	if inp.MockResult == nil { // the mock has no web to ask: an error naming what the script has to give
		return Result{Output: "WebSearch: the mock searches nothing: give the call a mock_result with the pages and findings to return", IsError: true}
	}
	r := inp.MockResult
	links := r.Links
	if links == nil {
		links = []tools.SearchLink{}
	}
	searches := 1
	if r.SearchCount != nil {
		searches = *r.SearchCount
	}
	return Result{
		Output: tools.SearchReport(inp.Query, links, r.Findings),
		ToolUseResult: map[string]any{
			"query":           inp.Query,
			"results":         []any{map[string]any{"tool_use_id": "srvtoolu_" + randomToolID(), "content": links}, r.Findings},
			"durationSeconds": r.DurationSecs,
			"searchCount":     searches,
		},
	}
}

// randomToolID is a made-up id of the kind the backend gives a search ("01RyFKQEWGC3...": 24 letters and digits).
func randomToolID() string {
	const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b)
}
