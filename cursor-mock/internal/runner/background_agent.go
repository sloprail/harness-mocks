package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sloprail/harness-mocks/cursor-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/cursor-mock/internal/toolexec"
	"github.com/sloprail/harness-mocks/internal/scenario"
	coresession "github.com/sloprail/harness-mocks/internal/session"
	"github.com/sloprail/harness-mocks/internal/subagents"
	"github.com/sloprail/harness-mocks/internal/tasks"
)

// taskInput is what the script's Task call carries: Cursor's own parameters,
// and the script that plays the sub-agent (the mock's: it has no model).
type taskInput struct {
	Description string `json:"description"`
	Prompt      string `json:"prompt"`
	Type        string `json:"subagent_type"`
	Background  bool   `json:"run_in_background"`
	Script      string `json:"script"`
}

// taskCall is the Task call as the stream shows it (recorded:
// runs/print-waits-for-background-agents).
func taskCall(in taskInput) toolexec.Call {
	kind := in.Type
	if kind == "" {
		kind = "generalPurpose"
	}
	return toolexec.Call{Kind: "taskToolCall", Args: map[string]any{
		"description": in.Description, "prompt": in.Prompt, "subagentType": map[string]any{kind: map[string]any{}}, "model": "default"}}
}

// launchTask carries out a Task call. The only one modeled is the one that
// runs in the background: the call is answered at once, with the sub-agent's
// id, and the sub-agent runs while the agent's turn goes on. Only preToolUse
// fires for it; no postToolUse does (recorded).
func (s *session) launchTask(ctx context.Context, tu scenario.ToolUse) {
	var in taskInput
	_ = json.Unmarshal(tu.Input, &in)
	call := taskCall(in)
	s.forward(startedFrame(s.id, tu.ID, call))
	s.tr.toolUse("Task", map[string]any{"description": in.Description, "prompt": in.Prompt,
		"subagent_type": in.Type, "run_in_background": in.Background})
	fail := func(msg string) { s.forward(errorFrame(s.id, tu.ID, call, msg)) }
	if missing := subagents.MissingFromDispatch(tu.Input); len(missing) > 0 {
		fail(fmt.Sprintf("Task: missing required parameter(s): %s", strings.Join(missing, ", ")))
		return
	}
	fields := hooks.ToolFields(hooks.Tool{Name: "Task", UseID: tu.ID, Input: map[string]any{
		"description": in.Description, "prompt": in.Prompt, "subagent_type": in.Type, "run_in_background": in.Background}})
	refused, msg := hooks.Refusal(s.hooks.Fire(ctx, hooks.PreToolUse, "Task", fields))
	s.named = true
	if refused {
		_, result := hooks.PreToolRefusal(msg)
		fail(result)
		return
	}
	if !in.Background || in.Script == "" {
		fail("Task: the mock models only a background sub-agent played by a script")
		return
	}
	t := tasks.NewTask(tasks.Agent, coresession.NewID())
	t.ToolUseID, t.Description, t.Meta = tu.ID, in.Description, in.Prompt
	s.bg.StartAgent(t, func(ctx context.Context) { s.runSubagent(ctx, t, in) })
	s.forward(completedFrame(s.id, tu.ID, call, map[string]any{"success": map[string]any{
		"conversationSteps": []any{}, "agentId": t.ID, "isBackground": true, "durationMs": "0",
		"backgroundReason": "SUBAGENT_BACKGROUND_REASON_AGENT_REQUEST"}}))
}

// runSubagent plays the sub-agent: its script's messages are what it said,
// and become its result; it keeps its own transcript. A sub-agent that asks for
// a tool is not modeled (adr/modeled-surface) and fails the run.
func (s *session) runSubagent(ctx context.Context, t *tasks.Task, in taskInput) {
	tr, err := newTranscript(s.cfg.Home, s.cfg.Dir, t.ID)
	if err != nil {
		t.Failure = err.Error()
		return
	}
	tr.user(in.Prompt)
	turn, err := scenario.RunTurn(ctx, in.Script, s.cfg.Dir, s.cfg.Environ, scenario.Input{Prompt: in.Prompt, SessionFile: tr.path})
	switch {
	case err != nil:
		t.Failure = err.Error()
	case turn.Tool != nil:
		t.Failure = "a sub-agent's tool calls are not modeled"
	default:
		for _, text := range turn.Texts {
			tr.text(text)
		}
		t.Result = strings.Join(turn.Texts, "")
	}
	tr.end()
	t.DurationMs = time.Since(t.Started).Milliseconds()
}
