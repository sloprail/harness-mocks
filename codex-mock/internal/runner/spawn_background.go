package runner

import (
	"context"
	"encoding/json"

	"github.com/sloprail/harness-mocks/codex-mock/internal/events"
	coresession "github.com/sloprail/harness-mocks/internal/session"
	"github.com/sloprail/harness-mocks/internal/subagents"
	"github.com/sloprail/harness-mocks/internal/tasks"
	"github.com/sloprail/harness-mocks/internal/toolcall"
)

// Codex's spawn_agent answers at once with a receipt, the sub-agent's id and a
// nickname; the sub-agent runs on its own thread, concurrently with the turn
// that launched it, its hooks naming it and firing under the session's id
// (recorded: runs/background-agent, runs/foreground-subagent-result). An agent
// that wants its report calls wait_agent (agent_wait.go).

// agents is the sub-agents the run has started, by thread id: what wait_agent
// waits on. One run per process, so one table.
var agents subagents.Waits

// spawnAgent makes the sub-agent's id and answers with its receipt; the stream
// shows its thread started and not yet running. It does not run until the
// call is answered (startBackground).
func (h toolHost) spawnAgent(c toolcall.Call) toolcall.Result {
	var in spawnInput
	_ = json.Unmarshal(c.Input, &in)
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(c.Input, &raw)
	if _, old := raw["background"]; old { // the mock's former parameter: spawn_agent always answers at once now
		return toolcall.Result{Output: "spawn_agent: the mock's 'background' parameter is gone (spawn_agent always answers at once); it is refused rather than ignored", Failed: true}
	}
	id := coresession.NewID()
	spawn := h.events.CollabStarted(agentTool, h.id, nil, in.Message)
	h.events.CollabCompleted(spawn, agentTool, h.id, []string{id}, in.Message,
		map[string]events.AgentState{id: {Status: "pending_init"}})
	b, _ := json.Marshal(struct {
		AgentID  string `json:"agent_id"`
		Nickname string `json:"nickname"`
	}{id, "Hypatia"})
	return toolcall.Result{Output: string(b)}
}

// startBackground lets the sub-agent of a dispatch run, once the agent has its
// receipt, in the session's background tasks.
//
// sr:provides background-agent/codex
func (h toolHost) startBackground(c toolcall.Call, receipt string) {
	var in spawnInput
	_ = json.Unmarshal(c.Input, &in)
	var r struct {
		AgentID string `json:"agent_id"`
	}
	_ = json.Unmarshal([]byte(receipt), &r)
	t := tasks.NewTask(tasks.Agent, r.AgentID)
	agents.Add(r.AgentID, subagents.Handle{Finished: t.Finished, Report: func() string { return t.Result }})
	h.bg.StartAgent(t, func(ctx context.Context) {
		rollout, err := h.createSub(r.AgentID)
		if err != nil {
			return
		}
		defer rollout.Close()
		t.Result = h.runSpawned(ctx, r.AgentID, rollout, in).LastAssistant
	})
}
