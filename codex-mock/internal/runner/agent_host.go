package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
const (
	spawnAgentTool = "spawn_agent"
	agentType      = "default"
)

// spawnInput is a spawn_agent call: the sub-agent's task, and (the mock's own
// parameter) the scenario script that drives it.
type spawnInput struct {
	Message string `json:"message"`
	Script  string `json:"script"`
}

// spawnAgent starts a sub-agent for the call, waits for it, and tells the
// agent how it ended, as the stream and the rollout record a spawn_agent
// followed by a wait. The sub-agent has a rollout of its own; its hooks are the
// session's.
//
// SubagentStart fires when it starts, with the sub-agent's own rollout as its
// transcript_path, and cannot refuse it: whatever the hook said, the
// sub-agent runs (recorded: runs/subagent-start-refused). SubagentStop fires
// when it stops, with the session's transcript_path, the sub-agent's own as
// agent_transcript_path, and its last message. The payload names no
// background tasks: Codex lists none (recorded: runs/subagent-lifecycle-hooks).
//
// sr:provides subagent-lifecycle-hooks/codex
func (h turnHost) spawnAgent(ctx context.Context, tu scenario.ToolUse) {
	var in spawnInput
	_ = json.Unmarshal(tu.Input, &in)
	subID := coresession.NewID()
	subRollout, err := session.Create(h.cfg.CodexHome, subID, h.cfg.Cwd, time.Now())
	if err != nil {
		h.rollout.ToolOutput(tu.ID, fmt.Sprintf("failed to start the sub-agent: %v", err))
		return
	}
	defer subRollout.Close()

	spawn := h.events.CollabStarted(spawnAgentTool, h.id, nil, in.Message)
	// the sub-agent is its own thread: its tools' hooks and its rollout are its own
	sub := *h.state
	sub.id, sub.turnID, sub.rollout = subID, coresession.NewID(), subRollout
	sub.events = events.New(io.Discard)
	invoker := *h.hooks
	invoker.Common.TranscriptPath = subRollout.Path
	sub.hooks = &invoker

	out := subagents.Execute(subagents.Hooks{
		Start: func() {
			invoker.Fire(ctx, hooks.SubagentStart, agentType, map[string]any{"turn_id": h.turnID, "agent_id": subID, "agent_type": agentType})
		},
		Stop: func(active bool, last string) (bool, string) {
			own := map[string]any{"turn_id": h.turnID, "agent_id": subID, "agent_type": agentType,
				"agent_transcript_path": subRollout.Path, "stop_hook_active": active, "last_assistant_message": last}
			h.hooks.Fire(ctx, hooks.SubagentStop, agentType, own) // what it answers is not read
			return false, ""
		},
	}, 0, func() subagents.Outcome { return runSubagent(ctx, &sub, in) })

	h.events.CollabCompleted(spawn, spawnAgentTool, h.id, []string{subID}, in.Message,
		map[string]events.AgentState{subID: {Status: "pending_init"}})
	wait := h.events.CollabStarted("wait", h.id, []string{subID}, nil)
	h.events.CollabCompleted(wait, "wait", h.id, []string{subID}, nil,
		map[string]events.AgentState{subID: {Status: "completed", Message: out.LastAssistant}})
	h.rollout.ToolOutput(tu.ID, fmt.Sprintf(`{"status":{%q:{"completed":%q}},"timed_out":false}`, subID, out.LastAssistant))
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

// agentTurnHost is the turn with the tool that starts a sub-agent: spawn_agent
// is carried out by spawnAgent, every other call as the turn does.
type agentTurnHost struct{ turnHost }

func (h agentTurnHost) Tool(ctx context.Context, tu scenario.ToolUse) {
	if tu.Name != spawnAgentTool {
		h.turnHost.Tool(ctx, tu)
		return
	}
	h.rollout.ToolCall(tu.ID, tu.Name, tu.Input)
	h.spawnAgent(ctx, tu)
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
