package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/sloprail/harness-mocks/codex-mock/internal/events"
	"github.com/sloprail/harness-mocks/codex-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/codex-mock/internal/session"
	"github.com/sloprail/harness-mocks/internal/scenario"
	coresession "github.com/sloprail/harness-mocks/internal/session"
	"github.com/sloprail/harness-mocks/internal/subagents"
	"github.com/sloprail/harness-mocks/internal/toolcall"
	"github.com/sloprail/harness-mocks/internal/turnloop"
)

// spawnAgentTool is the tool that starts a sub-agent; agentType is the type of
// one started without a profile (recorded: runs/subagent-lifecycle-hooks).
const agentType = "default"

// spawnInput is a spawn_agent call: the sub-agent's task, and (the mock's own
// parameter) the scenario script that drives it.
type spawnInput struct {
	Message string `json:"message"`
	Script  string `json:"script"`
}

// spawnAgent starts a sub-agent for a dispatch that passed the checks, waits
// for it, and gives what the agent is told of how it ended, as the stream records
// a spawn_agent followed by a wait. The sub-agent has a rollout of its own; its hooks are the
// session's.
//
// SubagentStart fires when it starts, with the sub-agent's own rollout as its
// transcript_path, and cannot refuse it: whatever the hook said, the
// sub-agent runs (recorded: runs/subagent-start-refused). SubagentStop fires
// when it stops, with the session's transcript_path, the sub-agent's own as
// agent_transcript_path, and its last message. The payload names no
// background tasks: Codex lists none (recorded: runs/subagent-lifecycle-hooks).
// A SubagentStop hook that blocks runs the sub-agent again with its reason as
// feedback until it lets it stop (recorded: runs/subagent-stop-block-loop).
//
// sr:provides subagent-lifecycle-hooks/codex
// sr:provides subagent-stop-block-loop/codex
func (h toolHost) spawnAgent(ctx context.Context, c toolcall.Call) toolcall.Result {
	var in spawnInput
	_ = json.Unmarshal(c.Input, &in)
	subID := coresession.NewID()
	subRollout, err := session.Create(h.cfg.CodexHome, subID, h.cfg.Cwd, time.Now())
	if err != nil {
		return toolcall.Result{Output: fmt.Sprintf("failed to start the sub-agent: %v", err), Failed: true}
	}
	defer subRollout.Close()

	spawn := h.events.CollabStarted(agentTool, h.id, nil, in.Message)
	// the sub-agent is its own thread: its tools' hooks and its rollout are its own
	sub := *h.state
	sub.id, sub.turnID, sub.rollout = subID, coresession.NewID(), subRollout
	sub.events = events.New(io.Discard)
	invoker := *h.hooks
	invoker.Common.TranscriptPath = subRollout.Path
	sub.hooks = &invoker

	out := subagents.Execute(subagents.Hooks{
		Start: func() {
			for _, o := range invoker.Fire(ctx, hooks.SubagentStart, agentType,
				map[string]any{"turn_id": h.turnID, "agent_id": subID, "agent_type": agentType}) {
				// what the hook prints, plain or as additionalContext, is developer context for the sub-agent (hooks#subagentstart)
				d := hooks.Interpret(hooks.SubagentStart, o)
				text := d.Context
				if text == "" && o.Exit == 0 && d.Error == "" && !strings.HasPrefix(strings.TrimSpace(o.Stdout), "{") {
					text = strings.TrimSpace(o.Stdout)
				}
				if text != "" {
					subRollout.Developer(text)
				}
			}
		},
		Stop: func(active bool, last string) (bool, string) {
			own := map[string]any{"turn_id": h.turnID, "agent_id": subID, "agent_type": agentType,
				"agent_transcript_path": subRollout.Path, "stop_hook_active": active, "last_assistant_message": last}
			for _, o := range h.hooks.Fire(ctx, hooks.SubagentStop, agentType, own) {
				// a block (exit 2, or a block decision) runs the sub-agent again, with the first reason as feedback
				switch d := hooks.Interpret(hooks.SubagentStop, o); {
				case d.Blocked:
					return true, d.BlockReason
				case d.Denied:
					return true, d.DenyReason
				}
			}
			return false, ""
		},
		OnRerun: func(reason string, _ int) {
			// the feedback is a user message in the sub-agent's own rollout, wrapped as Codex wraps a hook's prompt
			subRollout.User(fmt.Sprintf(`<hook_prompt hook_run_id="subagent-stop">%s</hook_prompt>`, reason))
		},
	}, 0, func() subagents.Outcome { return runSubagent(ctx, &sub, in) })

	h.events.CollabCompleted(spawn, agentTool, h.id, []string{subID}, in.Message,
		map[string]events.AgentState{subID: {Status: "pending_init"}})
	wait := h.events.CollabStarted("wait", h.id, []string{subID}, nil)
	h.events.CollabCompleted(wait, "wait", h.id, []string{subID}, nil,
		map[string]events.AgentState{subID: {Status: "completed", Message: out.LastAssistant}})
	return toolcall.Result{Output: fmt.Sprintf(`{"status":{%q:{"completed":%q}},"timed_out":false}`, subID, out.LastAssistant)}
}

// runSubagent drives the sub-agent's script as a turn of its own and reports
// its last message.
func runSubagent(ctx context.Context, sub *state, in spawnInput) subagents.Outcome {
	last, err := turnloop.Run(ctx, subHost{sub, in.Message}, turnloop.Params{
		Script: in.Script, Dir: sub.cfg.Cwd, Environ: sub.cfg.Environ, Prompt: in.Message})
	out := subagents.Outcome{LastAssistant: last, FinalText: last}
	if err != nil {
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
