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
	h.call = toolexec.FromScript(c.Name, c.Input)
	h.call.Request = h.s.requestID
	useID := c.ID
	if h.call.Kind == "mcpToolCall" {
		useID = coresession.NewID() // an MCP call's hooks name it by an id of their own (recorded: runs/hook-matchers-mcp)
	}
	h.tool = hooks.Tool{Name: h.call.Name(), Input: h.call.HookInput(h.s.cfg.Dir), UseID: useID}
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
	env := procexec.Env(h.s.cfg.Environ, childenv.Identity(h.s.id), childenv.Defaults())
	if h.call.Kind == "shellToolCall" && !h.s.cfg.Force {
		h.res = toolexec.Unapproved(h.call, h.s.cfg.Dir)
	} else if !h.read {
		h.res = toolexec.Execute(ctx, h.call, h.s.cfg.Dir, env)
		h.s.refuseResult(h.res)
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
