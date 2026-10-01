package runner

import (
	"context"
	"fmt"
	"strings"

	"github.com/sloprail/harness-mocks/codex-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/internal/scenario"
	"github.com/sloprail/harness-mocks/internal/toolcall"
)

// turnHost is Codex's side of a turn: its prompt and end-of-turn hooks, and
// its record of what was said.
type turnHost struct{ *state }

// SubmitPrompt fires UserPromptSubmit: the context the hooks add (plain text
// or additionalContext), and whether one blocked the prompt (exit 2, or a block
// decision).
func (h turnHost) SubmitPrompt(ctx context.Context) (string, bool) {
	var texts []string
	blocked := false
	own := map[string]any{"turn_id": h.turnID, "prompt": h.cfg.Prompt}
	for _, o := range h.hooks.Fire(ctx, hooks.UserPromptSubmit, "", own) {
		d := hooks.Interpret(hooks.UserPromptSubmit, o)
		blocked = blocked || d.Blocked || d.Denied
		if d.Context != "" {
			texts = append(texts, d.Context)
			h.rollout.Developer(d.Context)
		}
	}
	if !blocked {
		h.rollout.User(h.cfg.Prompt)
	}
	return strings.Join(texts, "\n"), blocked
}

// Say shows a message of the agent in the event stream and the rollout.
func (h turnHost) Say(text string) {
	h.events.AgentMessage(text)
	h.rollout.Assistant(text)
}

// Tool records the agent's call, and carries it out.
func (h turnHost) Tool(ctx context.Context, tu scenario.ToolUse) {
	h.rollout.ToolCall(tu.ID, tu.Name, tu.Input)
	toolcall.Run(ctx, toolHost{h.state}, toolcall.Call{ID: tu.ID, Name: tu.Name, Input: tu.Input},
		toolcall.Options{SeparateFailureHook: false})
}

// EndOfTurn fires Stop. A hook that blocks (exit 2, or a block decision) asks
// for the turn to continue; the first one's reason is the new prompt.
func (h turnHost) EndOfTurn(ctx context.Context, last string, continuing bool) (string, bool) {
	own := map[string]any{"turn_id": h.turnID, "stop_hook_active": continuing, "last_assistant_message": last}
	for _, o := range h.hooks.Fire(ctx, hooks.Stop, "", own) {
		d := hooks.Interpret(hooks.Stop, o)
		switch {
		case d.Blocked:
			return d.BlockReason, true
		case d.Denied:
			return d.DenyReason, true
		}
	}
	return "", false
}

// Continue records the reason that continues the turn, as the user message
// Codex makes of it.
func (h turnHost) Continue(reason string) {
	h.rollout.User(fmt.Sprintf(`<hook_prompt hook_run_id="stop">%s</hook_prompt>`, reason))
}

// SessionFile is the rollout the script reads.
func (h turnHost) SessionFile() string { return h.rollout.Path }
