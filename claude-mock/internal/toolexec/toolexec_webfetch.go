package toolexec

import (
	"encoding/json"

	"github.com/sloprail/harness-mocks/internal/tools"
)

// webFetchInput is the argument shape for the WebFetch tool: the url and the prompt of the real tool, and
// the mock's own mock_result, the answer a script gives the call (the mock reaches no web).
// sr:docs https://code.claude.com/docs/en/tools-reference#webfetch-tool-behavior
type webFetchInput struct {
	URL        string `json:"url"`
	Prompt     string `json:"prompt"`
	MockResult *struct {
		Result   string `json:"result"`
		Bytes    *int   `json:"bytes"`
		Code     *int   `json:"code"`
		CodeText string `json:"codeText"`
	} `json:"mock_result"`
}

// executeWebFetch answers a WebFetch the way claude 2.1.285 does (recorded: snapshots/runs/web-fetch-tool):
// a localhost or a host without a dot is refused, as an error that ran and failed; any other fetch is
// answered with the result the script gave the call, in the shape recorded: the answer as the text, and the
// toolUseResult of its size, status, answer and url. The mock fetches nothing: a call the script gave no
// result is an error result saying so.
//
// sr:provides web-fetch-tool/claude
func executeWebFetch(raw json.RawMessage) Result {
	var inp webFetchInput
	if err := json.Unmarshal(raw, &inp); err != nil || inp.URL == "" {
		return Result{Output: "WebFetch: missing or invalid 'url' field", IsError: true}
	}
	if tools.FetchRefused(inp.URL) {
		return failed(tools.FetchLocalMessage)
	}
	if inp.MockResult == nil { // the mock has no web to ask: an error naming what the script has to give
		return Result{Output: "WebFetch: the mock fetches no page: give the call a mock_result with the answer to return", IsError: true}
	}
	r := inp.MockResult
	bytes, code, codeText := len(r.Result), 200, r.CodeText
	if r.Bytes != nil {
		bytes = *r.Bytes
	}
	if r.Code != nil {
		code = *r.Code
	}
	if codeText == "" {
		codeText = "OK"
	}
	return Result{
		Output: r.Result,
		ToolUseResult: map[string]any{
			"bytes": bytes, "code": code, "codeText": codeText, "result": r.Result, "durationMs": 0, "url": inp.URL,
		},
	}
}
