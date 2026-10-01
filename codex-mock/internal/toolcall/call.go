// Package toolcall is one tool call of the agent: the before-tool hooks,
// refusing or running it, the events, and the after-tool hooks.
package toolcall

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/sloprail/harness-mocks/codex-mock/internal/events"
	"github.com/sloprail/harness-mocks/codex-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/codex-mock/internal/scenario"
	"github.com/sloprail/harness-mocks/codex-mock/internal/session"
	"github.com/sloprail/harness-mocks/codex-mock/internal/toolexec"
)

// Deps is what a call needs from its session.
type Deps struct {
	Hooks   *hooks.Invoker
	Env     []string // the environment of a shell command
	Dir     string
	Events  *events.Stream
	Session *session.File
	Stderr  io.Writer
	TurnID  string
}

// toolName is how hooks name the shell tool.
const toolName = "Bash"

// Run carries out a tool call the script asked for. A call a before-tool hook
// refuses does not run, fires no after-tool hook and shows no command in the
// event stream; the agent is told why as the call's error. A call that ran
// fires the after-tool hook, whatever its exit status; a hook that blocks it
// replaces the agent's result with its feedback, and the command has run.
//
// sr:docs https://developers.openai.com/codex/hooks#posttooluse
func Run(ctx context.Context, d Deps, tu scenario.ToolUse) {
	d.Session.ToolCall(tu.ID, tu.Name, tu.Input)
	var in struct {
		Command *string `json:"command"`
	}
	if tu.Name != toolName || json.Unmarshal(tu.Input, &in) != nil || in.Command == nil {
		d.Session.ToolOutput(tu.ID, "unsupported call: "+tu.Name)
		return
	}
	cmd := *in.Command
	own := map[string]any{"turn_id": d.TurnID, "tool_name": toolName, "tool_use_id": tu.ID,
		"tool_input": map[string]string{"command": cmd}}
	if refused, reason := refusal(ctx, d, own); refused {
		msg := fmt.Sprintf("Command blocked by PreToolUse hook: %s. Command: %s", reason, cmd)
		fmt.Fprintf(d.Stderr, "ERROR codex_core::tools::router: error=%s\n", msg)
		d.Session.ToolOutput(tu.ID, msg)
		return
	}
	id := d.Events.CommandStarted(cmd)
	res := toolexec.Bash(ctx, cmd, d.Dir, d.Env)
	d.Events.CommandCompleted(id, cmd, res.Output, res.ExitCode)
	own["tool_response"] = res.Output
	result := res.Output
	for _, o := range d.Hooks.Fire(ctx, hooks.PostToolUse, toolName, own) {
		dec := hooks.Interpret(hooks.PostToolUse, o)
		if feedback, ok := blockedWith(dec); ok {
			fmt.Fprintf(d.Stderr, "ERROR codex_core::tools::router: error=%s\n", feedback)
			result = feedback
		}
	}
	d.Session.ToolOutput(tu.ID, result)
}

func refusal(ctx context.Context, d Deps, own map[string]any) (bool, string) {
	var ds []hooks.Decision
	for _, o := range d.Hooks.Fire(ctx, hooks.PreToolUse, toolName, own) {
		ds = append(ds, hooks.Interpret(hooks.PreToolUse, o))
	}
	return hooks.Refusal(ds)
}

// blockedWith is the feedback of a hook that blocked: by exit status 2 or by a
// block decision.
func blockedWith(dec hooks.Decision) (string, bool) {
	switch {
	case dec.Blocked:
		return dec.BlockReason, true
	case dec.Denied:
		return dec.DenyReason, true
	}
	return "", false
}
