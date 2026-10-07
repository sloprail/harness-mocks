package toolexec

import (
	"encoding/json"
	"strings"

	"github.com/sloprail/harness-mocks/internal/subagents"
)

// required is each tool's required parameters, in the order Claude Code
// reports them.
var required = map[string][]string{
	"Bash":  {"command"},
	"Read":  {"file_path"},
	"Write": {"file_path", "content"},
	"Edit":  {"file_path", "old_string", "new_string"},
	"Glob":  {"pattern"},
	"Grep":  {"pattern"},
	// a sub-agent dispatch, whichever name it goes by (Task is Agent's old name)
	// sr:provides agent-input-validation/claude
	"Agent": subagents.DispatchRequired,
	"Task":  subagents.DispatchRequired,
}

// validationIssue is one entry of the issue list the transcript records for
// input that failed validation, in Claude Code's key order.
type validationIssue struct {
	Expected string   `json:"expected"`
	Code     string   `json:"code"`
	Path     []string `json:"path"`
	Message  string   `json:"message"`
}

// Required is the parameters Claude Code requires of a tool toolexec runs;
// nil for any other tool.
func Required(toolName string) []string { return required[toolName] }

// ValidationError is the tool_use_error Claude Code answers a call missing
// required parameters with: "<tool_use_error>InputValidationError: Read
// failed due to the following issue:\nThe required parameter `file_path` is
// missing</tool_use_error>", recording the issue list as the toolUseResult
// (claude 2.1.285, recorded: snapshots/runs/tool-invalid-input).
//
// sr:docs https://code.claude.com/docs/en/hooks#posttoolusefailure
func ValidationError(toolName string, missing []string) Result {
	var lines []string
	var issues []validationIssue
	for _, p := range missing {
		lines = append(lines, "The required parameter `"+p+"` is missing")
		issues = append(issues, validationIssue{
			Expected: "string", Code: "invalid_type", Path: []string{p},
			Message: "Invalid input: expected string, received undefined",
		})
	}
	head := toolName + " failed due to the following issue:\n"
	if len(issues) > 1 {
		head = toolName + " failed due to the following issues:\n"
	}
	recorded, _ := json.MarshalIndent(issues, "", "  ")
	return Result{
		Output:        "<tool_use_error>InputValidationError: " + head + strings.Join(lines, "\n") + "</tool_use_error>",
		IsError:       true,
		ToolUseResult: "InputValidationError: " + string(recorded),
	}
}
