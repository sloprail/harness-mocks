package runner

import (
	"context"

	"github.com/sloprail/harness-mocks/cursor-mock/internal/childenv"
	"github.com/sloprail/harness-mocks/cursor-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/cursor-mock/internal/toolexec"
	"github.com/sloprail/harness-mocks/internal/procexec"
	coresession "github.com/sloprail/harness-mocks/internal/session"
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
	read            bool // res is the read Before made
	prepared        bool // the call is built and its preToolUse has fired
	// contexts are what the call's after-tool hooks gave the agent, as its
	// completed frame carries them.
	contexts []any
}

func (h *toolHost) emit(line []byte) {
	if !h.quiet {
		h.s.forward(line)
	}
}

// Tool is the parameters a call to the named tool must carry.
func (h *toolHost) Tool(name string) ([]string, bool) { return toolexec.Required(name) }

// Before fires preToolUse and, for a shell command, beforeShellExecution and,
// for a read, beforeReadFile; the
// first stage whose hooks refuse ends it. A refusal is worded as Cursor words
// it for the stage (recorded: runs/pretool-refusal).
//
// sr:docs https://cursor.com/docs/hooks#pretooluse
func (h *toolHost) Before(ctx context.Context, c toolcall.Call) (bool, string) {
	if !h.prepared { // a call of a response of several had its preToolUse fired as it started (startedEarly)
		h.prepare(c)
		h.firePre(ctx, true)
	}
	h.s.named = true
	if h.refused {
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
	if h.call.Kind == "readToolCall" {
		// The file is read first, for the hook to be told what it holds; a read of
		// a file that is not there reaches no beforeReadFile hook (recorded:
		// runs/file-tools, runs/tool-failure). Any hook refusing it ends the call
		// (recorded: runs/before-read-refusal: exit 2, a JSON deny and invalid
		// JSON refuse it, exit 1 does not).
		h.res, h.read = toolexec.Execute(ctx, h.call, h.s.cfg.Dir, nil), true
		h.s.refuseResult(h.res)
		if r := h.res.Read; r != nil {
			own := map[string]any{"file_path": r.Path, "content": r.Content, "attachments": []any{}}
			if refused, msg := hooks.Refusal(h.s.hooks.Fire(ctx, hooks.BeforeReadFile, "Read", own)); refused {
				h.failure, h.result = hooks.ReadRefusal(msg)
				h.refused = true
				return true, h.result
			}
		}
	}
	return false, ""
}

// Execute runs the call, with the identity of the session in a shell
// command's environment, and prints its completed frame.
func (h *toolHost) Execute(ctx context.Context, _ toolcall.Call) toolcall.Result {
	env := procexec.Env(h.s.cfg.Environ, childenv.Identity(h.s.id, h.s.requestID, h.s.cfg.Version), childenv.Defaults())
	if h.call.Kind == "shellToolCall" && !h.s.cfg.Force {
		h.res = toolexec.Unapproved(h.call, h.s.cfg.Dir)
	} else if !h.read {
		h.res = toolexec.Execute(ctx, h.call, h.s.cfg.Dir, env)
		h.s.refuseResult(h.res)
		h.s.offload(&h.res)
	}
	return toolcall.Result{Output: h.res.Output, Failed: h.res.Failed}
}

// Answer ends the call on the stream: it ran, or a hook refused it, or it is a
// tool the mock does not have, or its input lacks parameters (no hook fires for
// the last two; their wording is the mock's own).
func (h *toolHost) Answer(c toolcall.Call, a toolcall.Answer) {
	switch a.Kind {
	case toolcall.Done:
		h.emit(completedFrame(h.s.id, c.ID, h.call, h.res.Frame, h.contexts))
	case toolcall.Refused:
		h.emit(rejectedFrame(h.s.id, c.ID, h.call, h.result, h.contexts))
	case toolcall.Unknown:
		h.emit(errorFrame(h.s.id, c.ID, toolexec.Call{Kind: "unknownToolCall"}, "Unknown tool: "+c.Name, nil))
	case toolcall.Invalid:
		h.emit(invalidFrame(h.s.id, c, a.Missing))
	}
}

// prepare builds the call and the tool its hooks are told of.
func (h *toolHost) prepare(c toolcall.Call) {
	h.call = toolexec.FromScript(c.Name, c.Input)
	h.call.Request = h.s.requestID
	if h.call.Kind == "readToolCall" {
		h.call.Args["path"] = h.s.resolveOffload(h.call.Path(h.s.cfg.Dir))
	}
	useID := c.ID
	if h.call.Kind == "mcpToolCall" || h.call.Kind == "shellToolCall" && !h.call.Background() {
		// an MCP call's hooks name it by an id of their own (recorded: runs/hook-matchers-mcp),
		// and so do a foreground command's; a command left in the background is
		// named by its call (recorded: runs/task-notifications-bg, runs/shell-exit-status)
		useID = coresession.NewID()
	}
	if h.call.HookID != "" && h.call.Kind != "mcpToolCall" { // a script names the id its recording's hooks gave the call
		useID = h.call.HookID
	}
	h.tool = hooks.Tool{Name: h.call.Name(), Input: h.call.HookInput(h.s.cfg.Dir), UseID: useID}
}

// firePre fires the call's preToolUse hooks; a refusal is the call's.
func (h *toolHost) firePre(ctx context.Context, name bool) {
	h.prepared = true
	if refused, msg := hooks.Refusal(h.s.hooks.Fire(ctx, hooks.PreToolUse, h.tool.Name, hooks.ToolFields(h.tool))); refused {
		h.failure, h.result = hooks.PreToolRefusal(msg)
		h.refused = true
	}
	if name { // the transcript is named once a call's preToolUse has fired, however it ended
		h.s.named = true
	}
}
