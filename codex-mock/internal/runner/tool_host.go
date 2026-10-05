package runner

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sloprail/harness-mocks/codex-mock/internal/hooks"
	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
	"github.com/sloprail/harness-mocks/internal/tasks"
	"github.com/sloprail/harness-mocks/internal/toolcall"
	"github.com/sloprail/harness-mocks/internal/tools"
)

// toolName is how hooks name the shell tool.
const toolName = "Bash"

// toolHost is Codex's side of a tool call: one shell tool, its hooks, and
// the words it tells the agent in.
type toolHost struct{ *state }

// Tool: the one tool the mock runs is the shell, which needs a command.
// A sub-agent dispatch needs a message (agent_tool.go).
func (h toolHost) Tool(name string) ([]string, bool) {
	if name == agentTool && canDispatch(posOf(h.id)) { // not offered to a sub-agent at the depth limit
		return agentRequired, true
	}
	if name == waitTool && canDispatch(posOf(h.id)) { // offered with it
		return waitRequired, true
	}
	return []string{"command"}, name == toolName || name == patchTool
}

func command(c toolcall.Call) string {
	var in struct {
		Command string `json:"command"`
	}
	_ = json.Unmarshal(c.Input, &in)
	return in.Command
}

func (h toolHost) payload(c toolcall.Call) map[string]any {
	if c.Name == agentTool || c.Name == waitTool {
		return h.byAgent(map[string]any{"turn_id": h.turnID, "tool_name": hookName(c), "tool_use_id": c.ID, "tool_input": c.Input})
	}
	return h.byAgent(map[string]any{"turn_id": h.turnID, "tool_name": fileOrHookName(c), "tool_use_id": c.ID,
		"tool_input": map[string]string{"command": command(c)}})
}

// Before fires PreToolUse and asks for the refusal of what the hooks decided.
func (h toolHost) Before(ctx context.Context, c toolcall.Call) (bool, string) {
	var ds []hooks.Decision
	for _, o := range h.hooks.Fire(ctx, hooks.PreToolUse, fileOrHookName(c), h.payload(c)) {
		ds = append(ds, hooks.Interpret(hooks.PreToolUse, o))
	}
	return hooks.Refusal(ds)
}

// Execute runs the command and shows it in the event stream.
func (h toolHost) Execute(ctx context.Context, c toolcall.Call) toolcall.Result {
	if c.Name == agentTool {
		return h.spawnAgent(c)
	}
	if c.Name == waitTool {
		return h.waitAgent(ctx, c)
	}
	if c.Name == patchTool {
		return h.applyPatch(c)
	}
	if option := h.unimplemented(c); option != "" {
		return refused(option)
	}
	cmd := command(c)
	argv := shellArgv(c, cmd)
	id := h.events.CommandStarted(cmd)
	var r tools.BashResult
	if y, yields := yieldTime(c); yields {
		var running bool
		if r, running = h.runYielding(ctx, c, argv, y); running {
			return toolcall.Result{Output: r.Output} // still running: no end to report
		}
	} else {
		r = tools.BashArgv(ctx, argv, h.cfg.Cwd, h.toolEnv)
	}
	r.Output = ttyOutput(c, r.Output)
	if tooLong(c, r.Output) {
		return refused("max_output_tokens below the command's output (the output is not truncated)")
	}
	h.events.CommandCompleted(id, cmd, r.Output, r.ExitCode)
	return toolcall.Result{Output: r.Output, Failed: r.Failed()}
}

// After fires PostToolUse, which Codex fires for every command that ran,
// whatever its exit status; a hook that blocks (exit 2, or a block decision)
// gives the agent its feedback in place of the result.
// sr:provides posttooluse-payload/codex
func (h toolHost) After(ctx context.Context, c toolcall.Call, r toolcall.Result, _ corehooks.AfterTool) (string, bool) {
	if !tasks.AfterHookFires(h.stillRunning(c.ID)) { // its PostToolUse comes when it ends, if ever
		return "", false
	}
	own := h.payload(c)
	own["tool_response"] = r.Output
	for _, o := range h.hooks.Fire(ctx, hooks.PostToolUse, fileOrHookName(c), own) {
		d := hooks.Interpret(hooks.PostToolUse, o)
		if d.Context != "" {
			h.rollout.Developer(d.Context)
		}
		switch {
		case d.Blocked:
			return d.BlockReason, true
		case d.Denied:
			return d.DenyReason, true
		}
	}
	return "", false
}

// Answer records what the agent was told, in Codex's words.
func (h toolHost) Answer(c toolcall.Call, a toolcall.Answer) {
	var text string
	switch a.Kind {
	case toolcall.Unknown:
		text = "unsupported call: " + c.Name
	case toolcall.Invalid:
		if c.Name == agentTool {
			text = spawnRefusal
			break
		}
		text = fmt.Sprintf("failed to parse function arguments: missing field `%s`", a.Missing[0])
	case toolcall.Refused:
		text = fmt.Sprintf("Command blocked by PreToolUse hook: %s. Command: %s", a.Reason, command(c))
		fmt.Fprintf(h.cfg.Stderr, "ERROR codex_core::tools::router: error=%s\n", text)
	case toolcall.Done:
		text = a.Result.Output
		if c.Name == patchTool && !a.Result.Failed {
			text = patchTold
		}
		if a.Replaced {
			text = a.Feedback
			fmt.Fprintf(h.cfg.Stderr, "ERROR codex_core::tools::router: error=%s\n", text)
		}
	}
	h.rollout.ToolOutput(c.ID, text)
	if c.Name == agentTool && a.Kind == toolcall.Done && !a.Replaced && !a.Result.Failed {
		h.startBackground(c, a.Result.Output) // a dispatch not waited for runs once it is answered
	}
}
