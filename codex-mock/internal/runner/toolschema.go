package runner

import "github.com/sloprail/harness-mocks/internal/toolspec"

// execParams are the options of an exec_command the mock implements, by the names a script gives
// them (the recordings call the command cmd). Each is one a recorded run shows the model pass:
// workdir (the mock runs in the run's directory only: another is refused when it is played), the
// yield time, max_output_tokens (which caps what the agent is told, and a command whose output
// is longer is refused: the mock does not truncate), shell (zsh, the one the recordings name),
// login (false with no shell, runs/exec-login-false; either with zsh) and tty.
var execParams = []toolspec.Param{
	{Name: "command", Recorded: "cmd", Type: toolspec.String, Required: true},
	{Name: "workdir", Type: toolspec.String},
	{Name: "yield_time_ms", Type: toolspec.Integer},
	{Name: "max_output_tokens", Type: toolspec.Integer},
	{Name: "shell", Type: toolspec.String, Values: []any{"zsh"}},
	{Name: "login", Type: toolspec.Boolean},
	{Name: "tty", Type: toolspec.Boolean},
}

// spawnParams: a sub-agent's first message; script is the mock's own, the sub-agent's scenario script.
// A call with no message is answered by Codex itself (recorded: runs/agent-input-validation).
var spawnParams = []toolspec.Param{
	{Name: "message", Type: toolspec.String, Required: true},
	{Name: "script", Type: toolspec.String, MockOnly: true},
}

// waitParams: the sub-agents to wait for (a script names them by the receipts their spawns
// returned) and how long.
var waitParams = []toolspec.Param{
	{Name: "targets", Type: toolspec.Array, Required: true},
	{Name: "timeout_ms", Type: toolspec.Integer},
}

// schema is the tools codex-mock implements, by the names a script gives them (Bash is how the
// mock and its hooks name exec_command) and the names the recordings give them.
var schema = toolspec.Schema{Harness: "codex", Tools: []toolspec.Tool{
	{Name: "Bash", Recorded: "exec_command", Params: execParams},
	{Name: "spawn_agent", Recorded: "multi_agent_v1__spawn_agent", Params: spawnParams,
		Answers: map[toolspec.Kind]string{toolspec.Missing: "agent-input-validation"}},
	{Name: "wait_agent", Recorded: "multi_agent_v1__wait_agent", Params: waitParams},
	{Name: "write_stdin", Params: []toolspec.Param{
		{Name: "session_id", Type: toolspec.Integer, Required: true},
		{Name: "yield_time_ms", Type: toolspec.Integer},
		{Name: "max_output_tokens", Type: toolspec.Integer},
	}},
	{Name: "apply_patch", Params: []toolspec.Param{{Name: "command", Recorded: "arg", Type: toolspec.String, Required: true}}},
}}

// Schema is the tools codex-mock implements.
func Schema() toolspec.Schema { return schema }
