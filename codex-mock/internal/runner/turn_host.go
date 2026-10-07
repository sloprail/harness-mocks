package runner

import (
	"context"
	"fmt"
	"strings"

	"github.com/sloprail/harness-mocks/codex-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/internal/scenario"
	"github.com/sloprail/harness-mocks/internal/toolcall"
	"github.com/sloprail/harness-mocks/internal/turnloop"
)

// turnHost is Codex's side of a turn: its prompt and end-of-turn hooks, and
// its record of what was said.
type turnHost struct{ *state }

// SubmitPrompt fires UserPromptSubmit: the context the hooks add (plain text
// or additionalContext), and whether one blocked the prompt (exit 2, or a block
// decision).
// sr:provides user-prompt-submit-hook/codex
// sr:provides hook-additional-context/codex
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
	h.rollout.CodeCall(tu.ID, codeOf(tu))
	toolcall.Run(ctx, toolHost{h.state}, toolcall.Call{ID: tu.ID, Name: tu.Name, Input: tu.Input},
		toolcall.Options{SeparateFailureHook: false, SilentFailure: failedPatch})
}

// EndOfTurn fires Stop. A hook that blocks (exit 2, or a block decision) asks
// for the turn to continue; the first one's reason is the new prompt.
// sr:provides stop-block-continuation/codex
// sr:provides stop-hook-payload/codex
func (h turnHost) EndOfTurn(ctx context.Context, last string, continuing bool) (string, bool) {
	var message any = last
	if last == "" { // recorded (runs/stop-no-message): an empty reply is a null message
		message = nil
	}
	own := map[string]any{"turn_id": h.turnID, "stop_hook_active": continuing, "last_assistant_message": message}
	var verdicts []turnloop.Verdict
	for _, o := range h.hooks.Fire(ctx, hooks.Stop, "", own) {
		d := hooks.Interpret(hooks.Stop, o)
		v := turnloop.Verdict{Halt: d.Halt}
		switch {
		case d.Blocked:
			v.Block, v.Reason = true, d.BlockReason
		case d.Denied:
			v.Block, v.Reason = true, d.DenyReason
		}
		verdicts = append(verdicts, v)
	}
	return turnloop.Resolve(verdicts)
}

// Continue records the reason that continues the turn, as the user message
// Codex makes of it.
func (h turnHost) Continue(reason string) {
	h.rollout.User(fmt.Sprintf(`<hook_prompt hook_run_id="stop">%s</hook_prompt>`, reason))
}

// contextMark begins a Notice that is context a background hook added, not a sub-agent's end.
const contextMark = "\x00context\x00"

// Notice is what the agent is to be told of (turnloop.Noticer): the context the hooks that ran in
// the background added, all that is due, and the next sub-agent's end, one at a time.
func (h turnHost) Notice(endOfTurn bool) (texts []string) {
	for _, c := range h.hooks.Due(h.prog.Started(), endOfTurn) {
		texts = append(texts, contextMark+c)
	}
	if text, ok := (toolHost{h.state}).nextNotice(); ok {
		texts = append(texts, text)
	}
	return texts
}

// Told records a context as a developer message, as the hook that adds it at the start does, and a
// sub-agent's end as the user message Codex makes of it.
func (h turnHost) Told(text string) {
	if c, ok := strings.CutPrefix(text, contextMark); ok {
		h.rollout.Developer(c)
		return
	}
	h.rollout.User(text)
}

// SessionFile is the rollout the script reads.
func (h turnHost) SessionFile() string { return h.rollout.Path }

// CapOverridden: Codex has no cap on end-of-turn blocks (the run passes none),
// so this is never called.
func (h turnHost) CapOverridden(int) {}
