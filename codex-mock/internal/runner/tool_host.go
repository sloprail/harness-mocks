package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

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
	defer h.prog.Move(1, 0) // started, hooks fired: what another agent's gate may wait for
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
	began := time.Now()
	var r tools.BashResult
	if y, yields := yieldTime(c); yields {
		var running bool
		if r, running = h.runYielding(ctx, c, argv, y); running {
			return toolcall.Result{Output: r.Output} // still running: no end to report
		}
	} else {
		r = tools.BashArgv(ctx, argv, h.cfg.Cwd, h.toolEnv)
	}
	if ctx.Err() != nil { // the user interrupted the turn: the command was stopped, and it is not reported as ended
		return toolcall.Result{Output: fmt.Sprintf("aborted by user after %.1fs", time.Since(began).Seconds())}
	}
	r.Output = ttyOutput(c, r.Output)
	if tooLong(c, r.Output) {
		return refused("max_output_tokens below the command's output (the output is not truncated)")
	}
	h.events.CommandCompleted(id, cmd, r.Output, r.ExitCode)
	return toolcall.Result{Output: r.Output, Failed: r.Failed(), Wall: time.Since(began), Ended: true}
}

// After fires PostToolUse, which Codex fires for every command that ran,
// whatever its exit status; a hook that blocks (exit 2, or a block decision)
// gives the agent its feedback in place of the result.
// sr:provides posttooluse-payload/codex
func (h toolHost) After(ctx context.Context, c toolcall.Call, r toolcall.Result, _ corehooks.AfterTool) (string, bool) {
	if ctx.Err() != nil { // an interrupted call fires no PostToolUse
		return "", false
	}
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
