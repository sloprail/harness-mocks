package runner

import (
	"context"

	"github.com/sloprail/harness-mocks/cursor-mock/internal/hooks"
	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
	"github.com/sloprail/harness-mocks/internal/toolcall"
)

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
		h.keepContext(ctx, hooks.PostToolUseFailure, own)
		return "", false
	}
	switch {
	case h.call.Kind == "shellToolCall":
		h.s.hooks.Fire(ctx, hooks.AfterShellExecution, h.call.Command(), map[string]any{
			"command": h.call.Command(), "output": h.res.Output, "duration": ms(h.res.Took), "sandbox": false})
	case h.call.Kind == "editToolCall" && !h.res.Failed:
		h.s.hooks.Fire(ctx, hooks.AfterFileEdit, "Write", map[string]any{"file_path": h.call.Path(h.s.cfg.Dir), "edits": h.res.Edits})
	}
	switch kind {
	case corehooks.AfterSuccess:
		own["tool_output"] = h.res.ToolOutput
		h.keepContext(ctx, hooks.PostToolUse, own)
	case corehooks.AfterFailure:
		own["error_message"], own["failure_type"], own["is_interrupt"] = h.res.ErrorMessage, "error", false
		h.keepContext(ctx, hooks.PostToolUseFailure, own)
	}
	return "", false
}

// keepContext fires the event's hooks for the call: what they add to the
// agent's context is kept, and shown on the call's completed frame.
func (h *toolHost) keepContext(ctx context.Context, e hooks.Event, own map[string]any) {
	ds := h.s.hooks.Fire(ctx, e, h.tool.Name, own)
	h.s.keep(ds)
	h.contexts = hooks.Contexts(e, ds)
}
