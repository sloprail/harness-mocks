package runner

import (
	"context"
	"encoding/json"

	"github.com/sloprail/harness-mocks/codex-mock/internal/events"
	coresession "github.com/sloprail/harness-mocks/internal/session"
	"github.com/sloprail/harness-mocks/internal/tasks"
	"github.com/sloprail/harness-mocks/internal/toolcall"
)

// A dispatch that is not waited for is what Codex's spawn_agent is: it answers
// at once with a receipt, the sub-agent's id and a nickname, and the sub-agent
// runs on its own thread, concurrently with the turn that launched it, its
// hooks naming it and firing under the session's id (recorded:
// runs/background-agent).

// spawnBackground makes the sub-agent's id and answers with its receipt; the
// stream shows its thread started and not yet running. It does not run until the
// call is answered (startBackground).
func (h toolHost) spawnBackground(in spawnInput) toolcall.Result {
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

// startBackground lets the sub-agent of a dispatch that is not waited for run,
// once the agent has its receipt, in the session's background tasks.
//
// sr:provides background-agent/codex
func (h toolHost) startBackground(c toolcall.Call, receipt string) {
	var in spawnInput
	_ = json.Unmarshal(c.Input, &in)
	if !in.Background {
		return
	}
	var r struct {
		AgentID string `json:"agent_id"`
	}
	_ = json.Unmarshal([]byte(receipt), &r)
	h.bg.StartAgent(tasks.NewTask(tasks.Agent, r.AgentID), func(ctx context.Context) {
		rollout, err := h.createSub(r.AgentID)
		if err != nil {
			return
		}
		defer rollout.Close()
		h.runSpawned(ctx, r.AgentID, rollout, in)
	})
}
