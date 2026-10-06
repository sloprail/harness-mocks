package runner

import "github.com/sloprail/harness-mocks/internal/toolspec"

// toolsDoc names the parameters of Claude Code's tools that the mock implements and no
// recorded run shows the model pass (a Glob path, an Agent model).
const toolsDoc = "https://code.claude.com/docs/en/tools-reference"

// agentParams are the parameters of an Agent (or Task) call as the model makes
// it (recorded: the Agent calls of runs/*); script is the mock's own. The only
// isolation the mock implements is a worktree.
var agentParams = []toolspec.Param{
	{Name: "description", Type: toolspec.String, Required: true},
	{Name: "prompt", Type: toolspec.String, Required: true},
	{Name: "subagent_type", Type: toolspec.String},
	{Name: "model", Type: toolspec.String, Doc: toolsDoc},
	{Name: "isolation", Type: toolspec.String, Values: []any{"worktree"}},
	{Name: "run_in_background", Type: toolspec.Boolean},
	{Name: "script", Type: toolspec.String, MockOnly: true},
}

// answers is the one kind Claude Code answers itself, a call missing a required
// parameter, which it answers with an InputValidationError: for a prompt (recorded:
// runs/agent-invalid-input), a description (runs/agent-invalid-description) and a
// Read's file_path (runs/tool-invalid-input).
func answers(run string) map[toolspec.Kind]string {
	return map[toolspec.Kind]string{toolspec.Missing: run}
}

// schema is the tools claude-mock implements, as a script names them: each
// parameter is one the recordings show the model pass. A tool the real claude has
// and the mock does not (Monitor, ...) is refused.
var schema = toolspec.Schema{Harness: "claude", Tools: []toolspec.Tool{
	{Name: "Bash", Params: []toolspec.Param{
		{Name: "command", Type: toolspec.String, Required: true},
		{Name: "description", Type: toolspec.String},
		{Name: "run_in_background", Type: toolspec.Boolean},
	}},
	{Name: "Read", Params: []toolspec.Param{
		{Name: "file_path", Type: toolspec.String, Required: true},
		{Name: "offset", Type: toolspec.Integer},
		{Name: "limit", Type: toolspec.Integer},
	}, Answers: answers("tool-invalid-input")},
	{Name: "Write", Params: []toolspec.Param{
		{Name: "file_path", Type: toolspec.String, Required: true},
		{Name: "content", Type: toolspec.String, Required: true},
	}},
	{Name: "Edit", Params: []toolspec.Param{
		{Name: "file_path", Type: toolspec.String, Required: true},
		{Name: "old_string", Type: toolspec.String, Required: true},
		{Name: "new_string", Type: toolspec.String, Required: true},
		{Name: "replace_all", Type: toolspec.Boolean},
	}},
	{Name: "Glob", Params: []toolspec.Param{
		{Name: "pattern", Type: toolspec.String, Required: true},
		{Name: "path", Type: toolspec.String, Doc: toolsDoc},
	}},
	// the only answer the mock gives is that no deferred tool matches (recorded:
	// runs/fgsub-tool-stats): it has none
	{Name: "ToolSearch", Params: []toolspec.Param{
		{Name: "query", Type: toolspec.String, Required: true},
		{Name: "max_results", Type: toolspec.Integer},
	}},
	{Name: "Agent", Params: agentParams, Answers: answers("agent-invalid-input")},
	{Name: "Task", Recorded: "Agent", Params: agentParams, Answers: answers("agent-invalid-input")},
	// the harness fills type, recipient and content beside to and message (recorded: runs/fgsub-maxturns)
	{Name: "SendMessage", Params: []toolspec.Param{
		{Name: "to", Type: toolspec.String, Required: true},
		{Name: "message", Type: toolspec.String},
		{Name: "type", Type: toolspec.String},
		{Name: "recipient", Type: toolspec.String},
		{Name: "content", Type: toolspec.String},
	}},
	{Name: "ScheduleWakeup", Params: []toolspec.Param{
		{Name: "delaySeconds", Type: toolspec.Integer},
		{Name: "prompt", Type: toolspec.String},
		{Name: "reason", Type: toolspec.String},
		{Name: "noop", Type: toolspec.Boolean},
		{Name: "stop", Type: toolspec.Boolean},
	}},
}}

// Schema is the tools claude-mock implements.
func Schema() toolspec.Schema { return schema }
