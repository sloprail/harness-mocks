package runner

import "github.com/sloprail/harness-mocks/internal/toolspec"

// taskParams are the parameters of a Task call as the model makes it (recorded:
// the Task calls of runs/*). Only its prompt is required: a call without a
// description runs (runs/agent-input-validation-description); script is the
// mock's own (taskInput).
var taskParams = []toolspec.Param{
	{Name: "description", Type: toolspec.String},
	{Name: "prompt", Type: toolspec.String, Required: true},
	{Name: "subagent_type", Type: toolspec.String},
	{Name: "model", Type: toolspec.String},
	{Name: "run_in_background", Type: toolspec.Boolean},
	{Name: "script", Type: toolspec.String, MockOnly: true},
}

// shellParams are the parameters of a Shell call (recorded: the Shell calls of
// runs/*). block_until_ms is the only option the mock implements: 0 leaves the
// command running, and the other values the recordings show (a command that
// ended in time) run it in the foreground; a foreground command that outlasts its
// limit moving to the background is not modeled (adr/modeled-surface).
var shellParams = []toolspec.Param{
	{Name: "command", Type: toolspec.String, Required: true},
	{Name: "description", Type: toolspec.String},
	{Name: "block_until_ms", Type: toolspec.Integer, Values: []any{0, 15000, 35000}},
}

// taskAnswers: a Task call without its prompt is answered by Cursor itself
// (recorded: runs/agent-input-validation).
var taskAnswers = map[toolspec.Kind]string{toolspec.Missing: "agent-input-validation"}

// schema is the tools cursor-mock implements, by the names a script gives them
// (the Claude Code names, with Cursor's Shell and Task too) and the names the
// recordings give them.
var schema = toolspec.Schema{Harness: "cursor", Tools: []toolspec.Tool{
	{Name: "Shell", Params: shellParams},
	{Name: "Bash", Recorded: "Shell", Params: shellParams},
	{Name: "Read", Params: []toolspec.Param{{Name: "file_path", Recorded: "path", Type: toolspec.String, Required: true}}},
	{Name: "Write", Params: []toolspec.Param{
		{Name: "file_path", Recorded: "path", Type: toolspec.String, Required: true},
		{Name: "content", Recorded: "contents", Type: toolspec.String, Required: true},
	}},
	{Name: "Edit", Recorded: "StrReplace", Params: []toolspec.Param{
		{Name: "file_path", Recorded: "path", Type: toolspec.String, Required: true},
		{Name: "old_string", Type: toolspec.String, Required: true},
		{Name: "new_string", Type: toolspec.String, Required: true},
	}},
	{Name: "Grep", Params: []toolspec.Param{{Name: "pattern", Type: toolspec.String, Required: true}}},
	{Name: "Delete", Params: []toolspec.Param{{Name: "file_path", Recorded: "path", Type: toolspec.String, Required: true}}},
	// a search of the harness's tool catalogue, by pattern, or the lookup of one
	// MCP tool the agent is about to call (recorded: runs/schedule-wakeup-ask, runs/hook-matchers-mcp)
	{Name: "GetDynamicTools", Params: []toolspec.Param{
		{Name: "pattern", Type: toolspec.String},
		{Name: "namespace", Type: toolspec.String},
		{Name: "toolName", Type: toolspec.String},
	}},
	{Name: "AwaitShell", Params: []toolspec.Param{
		{Name: "block_until_ms", Type: toolspec.Integer},
		{Name: "shell_id", Type: toolspec.String}, // a wait on a named task, which the mock refuses (runner/catalogue.go)
	}},
	// an MCP server's tool, named mcp__<server>__<tool> by a script, whose
	// arguments are the server's (recorded as CallDynamicTool)
	{Name: "mcp__*", Recorded: "CallDynamicTool", Open: true},
	{Name: "Task", Params: taskParams, Answers: taskAnswers},
	{Name: "Agent", Recorded: "Task", Params: taskParams, Answers: taskAnswers},
}}

// Schema is the tools cursor-mock implements.
func Schema() toolspec.Schema { return schema }
