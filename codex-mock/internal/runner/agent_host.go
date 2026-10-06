package runner

import (
	"context"
	"fmt"
	"io"

	"github.com/sloprail/harness-mocks/codex-mock/internal/events"
	"github.com/sloprail/harness-mocks/codex-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/codex-mock/internal/session"
	coresession "github.com/sloprail/harness-mocks/internal/session"
	"github.com/sloprail/harness-mocks/internal/subagents"
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

// runSpawned runs a started sub-agent to its end, with the hooks of its life.
// It is its own thread: its tools' hooks and its rollout are its own, and its
// hooks name it (recorded: runs/background-agent).
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
func (h toolHost) runSpawned(ctx context.Context, subID string, subRollout *session.File, in spawnInput) subagents.Outcome {
	sub := *h.state
	sub.id, sub.turnID, sub.rollout = subID, coresession.NewID(), subRollout
	sub.events = events.New(io.Discard)
	invoker := *h.hooks
	invoker.Later, invoker.Step = nil, nil // a sub-agent's hooks are all waited for: no recording shows otherwise
	invoker.Common.TranscriptPath, invoker.Common.AgentID, invoker.Common.AgentType = subRollout.Path, subID, agentType
	sub.hooks = &invoker
	sub.prog, sub.parent, sub.spawned = subagents.NewProgress(), h.prog, &subagents.SpawnLog{} // its own progress, read against the parent's

	return subagents.Execute(subagents.Hooks{
		Start: func() {
			for _, o := range invoker.Fire(ctx, hooks.SubagentStart, agentType,
				map[string]any{"turn_id": h.turnID, "agent_id": subID, "agent_type": agentType}) {
				// what the hook prints, plain or as additionalContext, is developer context for the sub-agent (hooks#subagentstart)
				if text := hooks.StartContext(o); text != "" {
					subRollout.Developer(text)
				}
			}
		},
		Stop: func(active bool, last string) (bool, string) {
			var message any = last
			if last == "" { // no message: null, as for the session's own stop (recorded: runs/subagent-stop-no-message)
				message = nil
			}
			own := map[string]any{"turn_id": h.turnID, "agent_id": subID, "agent_type": agentType,
				"agent_transcript_path": subRollout.Path, "stop_hook_active": active, "last_assistant_message": message}
			outs := h.hooks.Fire(ctx, hooks.SubagentStop, agentType, own)
			for _, o := range outs {
				if stopsFor(o.Stdout) { // continue:false outranks any block (hooks#subagentstop)
					return false, ""
				}
			}
			for _, o := range outs {
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
}
