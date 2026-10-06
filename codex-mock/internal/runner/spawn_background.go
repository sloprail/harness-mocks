package runner

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sloprail/harness-mocks/codex-mock/internal/events"
	coresession "github.com/sloprail/harness-mocks/internal/session"
	"github.com/sloprail/harness-mocks/internal/tasks"
	"github.com/sloprail/harness-mocks/internal/toolcall"
)

// Codex's spawn_agent answers at once with a receipt, the sub-agent's id and a
// nickname; the sub-agent runs on its own thread, concurrently with the turn
// that launched it, its hooks naming it and firing under the session's id
// (recorded: runs/background-agent, runs/foreground-subagent-result). An agent
// that wants its report calls wait_agent (agent_wait.go).

// spawnAgent makes the sub-agent's id and answers with its receipt; the stream
// shows its thread started and not yet running. It does not run until the
// call is answered (startBackground).
func (h toolHost) spawnAgent(c toolcall.Call) toolcall.Result {
	var in spawnInput
	_ = json.Unmarshal(c.Input, &in)
	id := coresession.NewID()
	h.spawned.Add(id)
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
	t.Owner = h.id // the agent that started it is the one told when it ends
	h.bg.StartAgent(t, func(ctx context.Context) {
		rollout, err := h.createSub(r.AgentID)
		if err != nil {
			t.Failure = fmt.Sprintf("failed to start the sub-agent: %v", err) // a wait refuses it, not an empty completion
			fmt.Fprintf(h.cfg.Stderr, "ERROR codex_mock: sub-agent %s: %s\n", r.AgentID, t.Failure)
			return
		}
		defer rollout.Close()
		t.Result = h.runSpawned(ctx, r.AgentID, rollout, in).LastAssistant
	})
}
