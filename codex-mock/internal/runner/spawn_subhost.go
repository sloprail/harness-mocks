package runner

import (
	"context"

	"github.com/sloprail/harness-mocks/internal/scenario"
	"github.com/sloprail/harness-mocks/internal/subagents"
	"github.com/sloprail/harness-mocks/internal/toolcall"
	"github.com/sloprail/harness-mocks/internal/turnloop"
)

// runSubagent drives the sub-agent's script as a turn of its own and reports
// its last message.
func runSubagent(ctx context.Context, sub *state, in spawnInput) subagents.Outcome {
	last, err := turnloop.Run(ctx, subHost{sub, in.Message}, turnloop.Params{Tools: Schema(),
		Script: in.Script, Dir: sub.cfg.Cwd, Environ: sub.cfg.Environ, Prompt: in.Message})
	out := subagents.Outcome{LastAssistant: last, FinalText: last}
	if err != nil {
		sub.refused.Set(err)
		out.Failure = err.Error()
	}
	return out
}

// subHost is a sub-agent's side of its turn: its task is its prompt, its tools
// are the session's, and no end-of-turn hook runs for it (SubagentStop does).
type subHost struct {
	*state
	task string
}

func (h subHost) SubmitPrompt(context.Context) (string, bool) {
	h.rollout.User(h.task)
	return "", false
}
func (h subHost) Say(text string) { h.rollout.Assistant(text) }
func (h subHost) Tool(ctx context.Context, tu scenario.ToolUse) {
	h.rollout.ToolCall(tu.ID, tu.Name, tu.Input)
	toolcall.Run(ctx, toolHost{h.state}, toolcall.Call{ID: tu.ID, Name: tu.Name, Input: tu.Input},
		toolcall.Options{SeparateFailureHook: false})
}
func (h subHost) EndOfTurn(context.Context, string, bool) (string, bool) { return "", false }
func (h subHost) Continue(string)                                        {}
func (h subHost) CapOverridden(int)                                      {}
func (h subHost) SessionFile() string                                    { return h.rollout.Path }
