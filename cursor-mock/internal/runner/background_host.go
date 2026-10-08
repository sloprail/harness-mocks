package runner

import (
	"context"

	"github.com/sloprail/harness-mocks/cursor-mock/internal/childenv"
	"github.com/sloprail/harness-mocks/cursor-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/cursor-mock/internal/toolexec"
	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
	"github.com/sloprail/harness-mocks/internal/procexec"
	"github.com/sloprail/harness-mocks/internal/scenario"
	"github.com/sloprail/harness-mocks/internal/toolcall"
)

// bgToolHost is the harness side of a Shell call left in the background: the
// call's hooks as any Shell's, but it starts the shell instead of running it,
// and no afterShellExecution follows (there is no end to report yet).
type bgToolHost struct{ *toolHost }

// runsInBackground reports whether a script's tool call is a Shell left running.
func runsInBackground(tu scenario.ToolUse) bool {
	return toolexec.FromScript(tu.Name, tu.Input).Background()
}

func (h *bgToolHost) Execute(ctx context.Context, c toolcall.Call) toolcall.Result {
	if !h.s.cfg.Force { // not approved: rejected, and not started, as any other command
		return h.toolHost.Execute(ctx, c)
	}
	env := procexec.Env(h.s.cfg.Environ, childenv.Identity(h.s.id, h.s.requestID, h.s.cfg.Version), childenv.Defaults())
	h.res = h.s.launch(h.call, h.tool.UseID, env)
	h.s.refuseResult(h.res)
	return toolcall.Result{Failed: h.res.Failed}
}

func (h *bgToolHost) After(ctx context.Context, c toolcall.Call, r toolcall.Result, kind corehooks.AfterTool) (string, bool) {
	if h.refused || h.res.Failed || !h.s.cfg.Force {
		return h.toolHost.After(ctx, c, r, kind)
	}
	own := hooks.ToolFields(h.tool)
	own["duration"] = ms(h.res.Took)
	own["tool_output"] = h.res.ToolOutput
	h.keepContext(ctx, hooks.PostToolUse, own)
	return "", false
}
