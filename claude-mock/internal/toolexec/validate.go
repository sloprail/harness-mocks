package toolexec

import (
	"encoding/json"
	"strings"
)

// required is each tool's required parameters, in the order Claude Code
// reports them.
var required = map[string][]string{
	"Bash":  {"command"},
	"Read":  {"file_path"},
	"Write": {"file_path", "content"},
	"Edit":  {"file_path", "old_string", "new_string"},
	"Glob":  {"pattern"},
}

// validationIssue is one entry of the issue list the transcript records for
// input that failed validation, in Claude Code's key order.
type validationIssue struct {
	Expected string   `json:"expected"`
	Code     string   `json:"code"`
	Path     []string `json:"path"`
	Message  string   `json:"message"`
}

// Validate is the tool_use_error a call whose input lacks a required
// parameter is answered with, before any hook fires and without running the
// tool; nil when the input is valid, or the tool is not one toolexec runs.
// Claude Code 2.1.285 answered a Read without file_path "<tool_use_error>
// InputValidationError: Read failed due to the following issue:\nThe required
// parameter `file_path` is missing</tool_use_error>", and recorded the issue
// list as the toolUseResult (recorded: snapshots/runs/tool-invalid-input).
//
// sr:docs https://code.claude.com/docs/en/hooks#posttoolusefailure
func Validate(toolName string, input json.RawMessage) *Result {
	params, ok := required[toolName]
	if !ok {
		return nil
	}
	var got map[string]json.RawMessage
	_ = json.Unmarshal(input, &got)
	var lines []string
	var issues []validationIssue
	for _, p := range params {
		if _, present := got[p]; present {
			continue
		}
		lines = append(lines, "The required parameter `"+p+"` is missing")
		issues = append(issues, validationIssue{
			Expected: "string", Code: "invalid_type", Path: []string{p},
			Message: "Invalid input: expected string, received undefined",
		})
	}
	if len(issues) == 0 {
		return nil
	}
	head := toolName + " failed due to the following issue:\n"
	if len(issues) > 1 {
		head = toolName + " failed due to the following issues:\n"
	}
	recorded, _ := json.MarshalIndent(issues, "", "  ")
	return &Result{
		Output:        "<tool_use_error>InputValidationError: " + head + strings.Join(lines, "\n") + "</tool_use_error>",
		IsError:       true,
		ToolUseResult: "InputValidationError: " + string(recorded),
	}
}
