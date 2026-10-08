package runner

import (
	"context"

	"github.com/sloprail/harness-mocks/cursor-mock/internal/toolexec"
	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
	"github.com/sloprail/harness-mocks/internal/scenario"
	"github.com/sloprail/harness-mocks/internal/toolcall"
)

// Tool carries out a call the script made: it is started and completed at once.
func (s *session) Tool(ctx context.Context, tu scenario.ToolUse) { s.Start(ctx, tu)() }

// Start is the first half of a call and returns the second (turnloop.Interleaver:
// Cursor was recorded starting all the calls of a turn before completing any,
// runs/task-stream-frames). The call is printed as started, then run through the
// shared order and recorded when it is completed. A write first reads the file
// it is about to change: a call of its own to the hooks (recorded:
// runs/tool-failure, runs/file-tools), which is not shown on the stream. A call
// refused at the depth limit is done at its start; a background Task is
// launched when it is completed.
func (s *session) Start(ctx context.Context, tu scenario.ToolUse) func() {
	if s.refusesTUITool(tu) {
		return func() {}
	}
	if s.refusesTaskAtTheLimit(ctx, tu) || s.refusesTaskModel(ctx, tu) {
		return func() {}
	}
	if finish, ok := s.startsHookless(ctx, tu); ok {
		return finish
	}
	if in, ok := startsBackgroundSubagent(tu); ok {
		return func() { s.launchSubagent(ctx, tu, in) }
	}
	if in, ok := dispatchesSubagent(tu); ok {
		return s.startSubagent(ctx, tu, in)
	}
	c := toolexec.FromScript(tu.Name, tu.Input)
	c.Request = s.requestID
	if c.Kind == "taskToolCall" && len(corehooks.RejectedInput(tu.Input, toolexec.TaskRequired())) > 0 {
		// a Task call that lacks its prompt is never started: only its completed
		// frame is on the stream, so what the agent said before it is not brought
		// out ahead of it (recorded: runs/agent-input-validation)
		s.tr.toolUse(tu.Name, c.Args)
		return func() { s.runTool(ctx, tu, false); s.named = true }
	}
	if c.Kind == "mcpToolCall" {
		if !s.cfg.ApproveMCPs {
			s.forward(startedFrame(s.id, tu.ID, c))
			s.tr.toolUse(tu.Name, c.Args)
			s.forward(errorFrame(s.id, tu.ID, c, s.refuseMsg("cursor-mock: an MCP tool call is modeled only with --approve-mcps (the mode it was recorded in)"), nil))
			return func() {}
		}
		s.readsMcpTool(ctx, tu, c)
	}
	s.forward(startedFrame(s.id, tu.ID, c))
	s.tr.toolUse(c.TranscriptName(tu.Name), c.TranscriptInput(s.cfg.Dir))
	if _, several := s.batched.Load(tu.ID); several && c.Kind != "taskToolCall" && c.Kind != "mcpToolCall" {
		// a response of several calls has every call's preToolUse fired as it starts, in
		// the order they are taken, before any of them runs (recorded: runs/task-stream-frames)
		h := &toolHost{s: s}
		h.prepare(toolcall.Call{ID: tu.ID, Name: tu.Name, Input: tu.Input})
		h.firePre(ctx, false) // not named yet: the calls beside it are told the same
		s.early.Store(tu.ID, h)
	}
	return func() {
		if c.Kind == "editToolCall" {
			path := map[string]any{"file_path": c.Args["path"]}
			s.runTool(ctx, scenario.ToolUse{ID: tu.ID, Name: "Read", Input: jsonLine(path)}, true)
		}
		s.runTool(ctx, tu, false)
		s.flushOwed()
	}
}

// runTool runs one call through the shared order; quiet leaves it off the
// stream.
func (s *session) runTool(ctx context.Context, tu scenario.ToolUse, quiet bool) {
	h := &toolHost{s: s, quiet: quiet}
	if early, ok := s.early.LoadAndDelete(tu.ID); ok && !quiet {
		h = early.(*toolHost)
	}
	var host toolcall.Host = h
	if runsInBackground(tu) {
		host = &bgToolHost{h}
	}
	toolcall.Run(ctx, host, toolcall.Call{ID: tu.ID, Name: tu.Name, Input: tu.Input},
		toolcall.Options{SeparateFailureHook: true, FailureOnRefusal: true})
}
