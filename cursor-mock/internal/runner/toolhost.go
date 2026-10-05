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

// toolHost is the harness side (toolcall.Host) of one tool call.
type toolHost struct {
	s     *session
	quiet bool // leave the call off the stream
	call  toolexec.Call
	tool  hooks.Tool
	// refused: a before-tool hook refused the call; failure and result are what
	// the failure hook and the agent are told.
	refused         bool
	failure, result string
	res             toolexec.Result
}

func (h *toolHost) emit(line []byte) {
	if !h.quiet {
		h.s.forward(line)
	}
}

// Tool is the parameters a call to the named tool must carry.
func (h *toolHost) Tool(name string) ([]string, bool) { return toolexec.Required(name) }

// Before fires preToolUse and, for a shell command, beforeShellExecution; the
// first stage whose hooks refuse ends it. A refusal is worded as Cursor words
// it for the stage (recorded: runs/pretool-refusal).
//
// sr:docs https://cursor.com/docs/hooks#pretooluse
func (h *toolHost) Before(ctx context.Context, c toolcall.Call) (bool, string) {
	h.call = toolexec.FromScript(c.Name, c.Input)
	h.tool = hooks.Tool{Name: h.call.Name(), Input: h.call.HookInput(h.s.cfg.Dir), UseID: c.ID}
	if refused, msg := hooks.Refusal(h.s.hooks.Fire(ctx, hooks.PreToolUse, h.tool.Name, hooks.ToolFields(h.tool))); refused {
		h.failure, h.result = hooks.PreToolRefusal(msg)
		h.refused = true
		h.s.named = true
		return true, h.result
	}
	h.s.named = true
	if h.call.Kind == "shellToolCall" {
		own := map[string]any{"command": h.call.Command(), "cwd": "", "sandbox": false}
		if refused, msg := hooks.Refusal(h.s.hooks.Fire(ctx, hooks.BeforeShellExecution, h.call.Command(), own)); refused {
			h.failure, h.result = hooks.ShellRefusal(msg)
			h.refused = true
			return true, h.result
		}
	}
	return false, ""
}

// Execute runs the call, with the identity of the session in a shell
// command's environment, and prints its completed frame.
func (h *toolHost) Execute(ctx context.Context, _ toolcall.Call) toolcall.Result {
	env := procexec.Env(h.s.cfg.Environ, childenv.Identity(h.s.id), childenv.Defaults())
	if h.call.Kind == "shellToolCall" && !h.s.cfg.Force {
		h.res = toolexec.Unapproved(h.call, h.s.cfg.Dir)
	} else {
		h.res = toolexec.Execute(ctx, h.call, h.s.cfg.Dir, env)
	}
	h.emit(completedFrame(h.s.id, h.tool.UseID, h.call, h.res.Frame))
	return toolcall.Result{Output: h.res.Output, Failed: h.res.Failed}
}

// After fires the hooks that follow a call: for one that ran, the events of
// its tool (afterShellExecution, afterFileEdit) and then the success hook with
// the tool's output, or the failure hook with its error; for a refused call,
// the failure hook with permission_denied. No hook's output changes what the
// agent is told of the result; the additional_context of the success and failure
// hooks is added to what the agent knows (recorded: runs/additional-context).
//
// sr:provides tool-failure-hook/cursor
// sr:provides posttooluse-payload/cursor
// sr:docs https://cursor.com/docs/hooks#posttoolusefailure
func (h *toolHost) After(ctx context.Context, _ toolcall.Call, _ toolcall.Result, kind corehooks.AfterTool) (string, bool) {
	own := hooks.ToolFields(h.tool)
	own["duration"] = ms(h.res.Took)
	if h.refused {
		own["error_message"], own["failure_type"], own["is_interrupt"] = h.failure, "permission_denied", false
		h.s.keep(h.s.hooks.Fire(ctx, hooks.PostToolUseFailure, h.tool.Name, own))
		return "", false
	}
	switch {
	case h.call.Kind == "shellToolCall":
		h.s.hooks.Fire(ctx, hooks.AfterShellExecution, h.call.Command(), map[string]any{
			"command": h.call.Command(), "output": h.res.Output, "duration": ms(h.res.Took), "sandbox": false})
	case h.call.Kind == "editToolCall" && !h.res.Failed:
		h.s.hooks.Fire(ctx, hooks.AfterFileEdit, "Write", map[string]any{"file_path": h.call.Path(h.s.cfg.Dir), "edits": h.res.Edits})
	case h.call.Kind == "readToolCall" && h.res.Read != nil:
		h.s.hooks.Fire(ctx, hooks.BeforeReadFile, "Read", map[string]any{"file_path": h.res.Read.Path, "content": h.res.Read.Content, "attachments": []any{}})
	}
	switch kind {
	case corehooks.AfterSuccess:
		own["tool_output"] = h.res.ToolOutput
		h.s.keep(h.s.hooks.Fire(ctx, hooks.PostToolUse, h.tool.Name, own))
	case corehooks.AfterFailure:
		own["error_message"], own["failure_type"], own["is_interrupt"] = h.res.ErrorMessage, "error", false
		h.s.keep(h.s.hooks.Fire(ctx, hooks.PostToolUseFailure, h.tool.Name, own))
	}
	return "", false
}

// Answer tells the agent how a call that did not run ended: refused by a hook,
// a tool the mock does not have, or input that lacks parameters (no hook
// fires for the last two; their wording is the mock's own).
func (h *toolHost) Answer(c toolcall.Call, a toolcall.Answer) {
	switch a.Kind {
	case toolcall.Refused:
		h.emit(rejectedFrame(h.s.id, c.ID, h.call, h.result))
	case toolcall.Unknown:
		h.emit(errorFrame(h.s.id, c.ID, toolexec.Call{Kind: "unknownToolCall"}, "Unknown tool: "+c.Name))
	case toolcall.Invalid:
		h.emit(invalidFrame(h.s.id, c, a.Missing))
	}
}
