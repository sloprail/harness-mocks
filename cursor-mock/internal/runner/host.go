package runner

import (
	"context"

	"github.com/sloprail/harness-mocks/cursor-mock/internal/toolexec"
	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
	"github.com/sloprail/harness-mocks/internal/scenario"
	coresession "github.com/sloprail/harness-mocks/internal/session"
	"github.com/sloprail/harness-mocks/internal/toolcall"
)

// SubmitPrompt fires nothing in print mode (recorded: cursor-agent -p never fires
// beforeSubmitPrompt); a TUI session fires it (interactive.go).
func (s *session) SubmitPrompt(ctx context.Context) (string, bool) {
	if s.cfg.Interactive {
		return s.submitPrompt(ctx)
	}
	return "", false
}

// Say prints what the agent said, and records it; a TUI session tells it to
// afterAgentResponse when the turn ends.
func (s *session) Say(text string) {
	s.texts = append(s.texts, text)
	s.tr.text(text)
	s.pending = append(s.pending, text)
	if s.firesStop() {
		s.said = append(s.said, text)
	}
}

// EndOfTurn fires no hook in print mode (recorded: cursor-agent -p never fires stop), unless
// run with the opt-in; what can continue the turn is a background shell's end (afterTurn).
//
// What the agent said in the turn is not brought out when the turn ends: a
// turn a finished background shell gives the agent follows, and everything said
// since the last call comes out as one frame at the end of the run, after the
// task's notification (recorded: runs/background-bash-start,
// runs/bg-bash-reaped-at-exit).
func (s *session) EndOfTurn(ctx context.Context, _ string, _ bool) (string, bool) {
	if s.cfg.Interactive {
		return s.stopped(ctx)
	}
	if s.cfg.Stop && s.owner == "" { // the opt-in: the TUI's stop, in print mode (interactive.go)
		if follow, again := s.stopped(ctx); again {
			return follow, true
		}
	}
	if s.owner != "" { // a sub-agent ends with its final response: its parent goes on
		return "", false
	}
	return s.afterTurn(ctx)
}

// Continue records the turn a finished background shell gives the agent, as the
// user message it is in the transcript.
func (s *session) Continue(prompt string) {
	s.tr.user(prompt)
	s.requestID, s.modelN = coresession.NewID(), 0 // a turn of its own is a model request of its own
	if s.stopFollowUp {                            // the follow-up a stop hook gave
		s.followedUp()
	}
}

// CapOverridden: a stop hook asked for more than the cap; the turn ends.
func (s *session) CapOverridden(int) {}

// SessionFile is the conversation's transcript so far.
func (s *session) SessionFile() string { return s.tr.path }

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
	s.tr.toolUse(tu.Name, c.Args)
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
