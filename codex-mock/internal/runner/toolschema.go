package runner

import "github.com/sloprail/harness-mocks/internal/toolspec"

// schema is the tools codex-mock implements, by the names a script gives them
// and the names the recordings give them (exec_command's cmd, a spawn_agent's
// message). The shell tool takes a command and how long to wait for it
// (yield_time_ms); workdir, max_output_tokens, login, shell and tty are the other
// options the recorded calls carry, and the mock reads none of them yet: they
// are declared so that the recorded runs replay as they did, and each leaves the
// schema or gains its Values when the mock implements or refuses it. The mock's own parameters of
// spawn_agent are the script that plays the sub-agent and whether it is waited
// for; task_name and fork_turns are the dispatch's other recorded parameters, read
// by nothing yet. A spawn_agent without its message is answered by Codex itself (recorded:
// runs/agent-input-validation).
var schema = toolspec.Schema{Harness: "codex", Tools: []toolspec.Tool{
	{Name: toolName, Recorded: "exec_command", Params: []toolspec.Param{
		{Name: "command", Recorded: "cmd", Type: toolspec.String, Required: true},
		{Name: "yield_time_ms", Type: toolspec.Integer},
		{Name: "workdir", Type: toolspec.String},
		{Name: "max_output_tokens", Type: toolspec.Integer},
		{Name: "login", Type: toolspec.Boolean},
		{Name: "shell", Type: toolspec.String},
		{Name: "tty", Type: toolspec.Boolean},
	}},
	{Name: patchTool, Params: []toolspec.Param{{Name: "command", Type: toolspec.String, Required: true}}},
	{Name: agentTool, Params: []toolspec.Param{
		{Name: "message", Type: toolspec.String, Required: true},
		{Name: "task_name", Type: toolspec.String},
		{Name: "fork_turns", Type: toolspec.String},
		{Name: "script", Type: toolspec.String, MockOnly: true},
		{Name: "background", Type: toolspec.Boolean, MockOnly: true},
	}, Answers: map[toolspec.Kind]string{toolspec.Missing: "agent-input-validation"}},
}}

// Schema is the tools codex-mock implements.
func Schema() toolspec.Schema { return schema }
