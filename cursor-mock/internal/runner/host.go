package runner

import (
	"context"

	"github.com/sloprail/harness-mocks/cursor-mock/internal/toolexec"
	"github.com/sloprail/harness-mocks/internal/scenario"
	"github.com/sloprail/harness-mocks/internal/toolcall"
)

// SubmitPrompt fires nothing: cursor-agent in print mode was recorded not
// firing beforeSubmitPrompt, so no hook blocks the prompt or adds context.
func (s *session) SubmitPrompt(context.Context) (string, bool) { return "", false }

// Say prints what the agent said, and records it.
func (s *session) Say(text string) {
	s.texts = append(s.texts, text)
	s.tr.text(text)
	s.forward(assistantFrame(s.id, text))
}

// EndOfTurn fires no hook: cursor-agent in print mode was recorded not firing
// the stop hook, so nothing blocks the end of the turn. What can continue it is
// a background shell's end (afterTurn).
func (s *session) EndOfTurn(ctx context.Context, _ string, _ bool) (string, bool) {
	return s.afterTurn(ctx)
}

// Continue records the turn a finished background shell gives the agent, as the
// user message it is in the transcript.
func (s *session) Continue(prompt string) { s.tr.user(prompt) }

// CapOverridden is never asked for: no end-of-turn hook blocks, so there is no
// cap to reach.
func (s *session) CapOverridden(int) {}

// SessionFile is the conversation's transcript so far.
func (s *session) SessionFile() string { return s.tr.path }

// Tool carries out a call the script made: it is printed as started, run
// through the shared order of a tool call, and recorded. A write first reads
// the file it is about to change: a call of its own to the hooks (recorded:
// runs/tool-failure, runs/file-tools), which is not shown on the stream.
func (s *session) Tool(ctx context.Context, tu scenario.ToolUse) {
	if tu.Name == "Task" {
		s.launchTask(ctx, tu)
		return
	}
	c := toolexec.FromScript(tu.Name, tu.Input)
	s.forward(startedFrame(s.id, tu.ID, c))
	s.tr.toolUse(tu.Name, c.Args)
	if c.Kind == "editToolCall" {
		path := map[string]any{"file_path": c.Args["path"]}
		s.runTool(ctx, scenario.ToolUse{ID: tu.ID + "-read", Name: "Read", Input: jsonLine(path)}, true)
	}
	s.runTool(ctx, tu, false)
}

// runTool runs one call through the shared order; quiet leaves it off the
// stream.
func (s *session) runTool(ctx context.Context, tu scenario.ToolUse, quiet bool) {
	h := &toolHost{s: s, quiet: quiet}
	var host toolcall.Host = h
	if runsInBackground(tu) {
		host = &bgToolHost{h}
	}
	toolcall.Run(ctx, host, toolcall.Call{ID: tu.ID, Name: tu.Name, Input: tu.Input},
		toolcall.Options{SeparateFailureHook: true, FailureOnRefusal: true})
}
