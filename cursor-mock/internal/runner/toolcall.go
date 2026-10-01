package runner

import (
	"context"

	"github.com/sloprail/harness-mocks/cursor-mock/internal/childenv"
	"github.com/sloprail/harness-mocks/cursor-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/cursor-mock/internal/toolexec"
	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
	"github.com/sloprail/harness-mocks/internal/procexec"
	"github.com/sloprail/harness-mocks/internal/toolcall"
)

// refusal is how a refused call is reported: the failure hook's error_message
// and the result the agent is given.
type refusal struct{ failure, result string }

// Run carries out one tool call the script asked for. preToolUse fires first,
// then, for a shell command, beforeShellExecution; the first hook to refuse
// ends the call, which is reported as a rejected tool call and a failure hook
// with permission_denied. A call that runs reports its completed frame, the
// events of its tool (afterShellExecution, afterFileEdit), and then the
// success hook, or the failure hook when it failed.
//
// sr:provides tool-failure-hook/cursor
// sr:docs https://cursor.com/docs/hooks#posttoolusefailure
func (s *session) Run(ctx context.Context, p pending) error {
	// A write first reads the file it is about to change: a call of its own to
	// the hooks (recorded: runs/tool-failure, runs/file-tools), which is not
	// shown on the stream.
	if p.call.Kind == "editToolCall" {
		s.run(ctx, pending{call: toolexec.Call{Kind: "readToolCall", Args: map[string]any{"path": p.call.Args["path"]}}, id: p.id + "-read"}, true)
	}
	s.run(ctx, p, false)
	return nil
}

// run takes one call through its hooks and its tool; quiet leaves the call off
// the stream.
func (s *session) run(ctx context.Context, p pending, quiet bool) {
	emit := func(line []byte) {
		if !quiet {
			s.forward(line)
		}
	}
	tool := hooks.Tool{Name: p.call.Name(), Input: p.call.HookInput(s.cfg.Dir), UseID: p.id}
	var ref refusal
	gates := []toolcall.Gate{func(ctx context.Context) (bool, string) {
		refused, msg := hooks.Decide(s.hooks.Fire(ctx, hooks.PreToolUse, hooks.ToolFields(tool)))
		ref.failure, ref.result = hooks.PreToolRefusal(msg)
		return refused, msg
	}}
	if p.call.Kind == "shellToolCall" {
		gates = append(gates, func(ctx context.Context) (bool, string) {
			own := map[string]any{"command": p.call.Command(), "cwd": "", "sandbox": false}
			refused, msg := hooks.Decide(s.hooks.Fire(ctx, hooks.BeforeShellExecution, own))
			ref.failure, ref.result = hooks.ShellRefusal(msg)
			return refused, msg
		})
	}
	var res toolexec.Result
	outcome, _ := toolcall.Run(ctx, gates, func(ctx context.Context) corehooks.ToolOutcome {
		env := procexec.Env(s.cfg.Environ, childenv.Identity(s.id), childenv.Defaults())
		res = toolexec.Execute(ctx, p.call, s.cfg.Dir, env)
		return res.Outcome
	})
	if outcome == corehooks.ToolRefused {
		emit(rejectedFrame(s.id, p, ref.result))
		s.afterTool(ctx, tool, corehooks.AfterRefusedTool(true), res, "permission_denied", ref.failure)
		return
	}
	emit(completedFrame(s.id, p, res.Frame))
	s.toolEvents(ctx, p.call, res)
	s.afterTool(ctx, tool, corehooks.AfterToolHook(outcome), res, "error", res.ErrorMessage)
}

// toolEvents fires the events a tool raises of its own once it has run.
func (s *session) toolEvents(ctx context.Context, c toolexec.Call, res toolexec.Result) {
	switch {
	case c.Kind == "shellToolCall":
		s.hooks.Fire(ctx, hooks.AfterShellExecution, map[string]any{
			"command": c.Command(), "output": res.Output, "duration": ms(res.Took), "sandbox": false})
	case c.Kind == "editToolCall" && res.Outcome == corehooks.ToolSucceeded:
		s.hooks.Fire(ctx, hooks.AfterFileEdit, map[string]any{"file_path": c.Path(s.cfg.Dir), "edits": res.Edits})
	}
}

// afterTool fires the hook a call's outcome calls for: the success hook with
// the tool's output, or the failure hook with its error.
func (s *session) afterTool(ctx context.Context, t hooks.Tool, which corehooks.AfterTool, res toolexec.Result, failureType, message string) {
	own := hooks.ToolFields(t)
	own["duration"] = ms(res.Took)
	switch which {
	case corehooks.AfterSuccess:
		own["tool_output"] = res.ToolOutput
		s.hooks.Fire(ctx, hooks.PostToolUse, own)
	case corehooks.AfterFailure:
		own["error_message"], own["failure_type"], own["is_interrupt"] = message, failureType, false
		s.hooks.Fire(ctx, hooks.PostToolUseFailure, own)
	}
}
