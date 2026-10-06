package replay

import (
	"fmt"
	"time"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// turns are what an agent did, with the id each of its calls had, so that the
// agent a call started can be attached to it.
type turns struct {
	agent   core.Agent
	ids     []string // the id of each of agent.Calls
	agentID string   // for a sub-agent: its own id, as its file names it
}

// modelTurns are the calls the model made in one recorded transcript, in
// order, and its final answer. A transcript holds one record per content block
// the model wrote (its thinking, its text, each tool call), then a user record
// per tool result, so a call is a tool_use block, what the model said just
// before it is the text block ahead of it, and the final answer is the text
// that no call follows. The tools are mapped onto the unified ones here; what
// the adapter cannot map is an error, never a guess.
func modelTurns(records []map[string]any) (turns, error) {
	var t turns
	var said *string
	var saidAt time.Time
	var before []string // what it said earlier still, ahead of the same call
	records = withoutForkContext(records)
	compacts := newCompactions(records)
	agents := callAgentIDs(records)
	done := map[string]time.Time{} // when each call's result was given back
	for _, rec := range records {
		if rec["type"] == "user" {
			for id := range resultIDs(rec) {
				done[id] = stampOf(rec)
			}
		}
		if rec["type"] == "user" && isStopFeedback(rec) && said == nil {
			return turns{}, fmt.Errorf("a Stop hook's feedback came to a model that had said nothing")
		}
		if endsTurn(rec) && said != nil {
			// a turn ends and the harness goes on from it (a Stop hook's feedback, a task's notification):
			// what the model said last was the answer that ended it, and the model answers again
			t.agent.Calls = append(t.agent.Calls, core.Call{Tool: toolReply, At: saidAt, Input: map[string]any{"text": *said}, SaidBefore: before})
			t.ids = append(t.ids, "")
			said, before = nil, nil
		}
		if c, ok := compacts.see(rec); ok {
			t.agent.Calls = append(t.agent.Calls, c)
			t.ids = append(t.ids, "")
		}
		if rec["type"] == "user" && isNotificationTurn(rec) && said != nil {
			// a finished background agent's notification starts a new turn: what the model said before it was its answer
			// to the turn that ended, and it answers the notification in turn
			t.agent.Calls = append(t.agent.Calls, core.Call{Tool: toolReply, At: saidAt, Input: map[string]any{"text": *said}})
			t.ids = append(t.ids, "")
			said = nil
			continue
		}
		if rec["type"] == "user" && isNudge(rec) && said == nil {
			// the model's response had no visible output (only thinking): the harness nudged it to go on
			t.agent.Calls = append(t.agent.Calls, core.Call{Tool: toolReply, At: stampOf(rec), Input: map[string]any{"text": "", "silent": true}})
			t.ids = append(t.ids, "")
			continue
		}
		if rec["type"] != "assistant" {
			continue
		}
		msg, _ := rec["message"].(map[string]any)
		blocks, _ := msg["content"].([]any)
		for _, b := range blocks {
			block, _ := b.(map[string]any)
			switch block["type"] {
			case "text":
				if said != nil {
					before = append(before, *said)
				}
				text, _ := block["text"].(string)
				said, saidAt = &text, stampOf(rec)
			case "tool_use":
				call, err := unify(block)
				if err != nil {
					return turns{}, err
				}
				call.Said, call.SaidBefore, call.At = said, before, stampOf(rec)
				call.Ref = agents[blockID(block)] // the agent a spawn started, as a message to it names it
				said, before = nil, nil
				id, _ := block["id"].(string)
				if n := len(t.ids); n > 0 && t.ids[n-1] != "" {
					if _, answered := done[t.ids[n-1]]; !answered {
						t.agent.Calls[n-1].More = true // sent in one message with this one: no result between them
					}
				}
				t.agent.Calls = append(t.agent.Calls, call)
				t.ids = append(t.ids, id)
			}
		}
	}
	if said != nil {
		t.agent.Final, t.agent.FinalAt = *said, saidAt
	}
	for i, id := range t.ids {
		t.agent.Calls[i].Done = done[id]
	}
	return t, nil
}

// attachSubagents is the main agent's calls with each spawn's sub-agent attached,
// and theirs in turn, found by the id of the call that started them. A spawn
// with no recorded sub-agent keeps none: the call itself was refused.
func attachSubagents(t turns, subs map[string]turns, early, late map[string]bool) core.Agent {
	agent := core.Agent{Calls: append([]core.Call(nil), t.agent.Calls...), Final: t.agent.Final, FinalAt: t.agent.FinalAt}
	for i, c := range agent.Calls {
		agent.Calls[i].ExecEarly = early[t.ids[i]]
		if late[t.ids[i]] { // the harness started the sub-agent after the call's PostToolUse, in this sample
			agent.Calls[i].Input = withKey(agent.Calls[i].Input, "mock_start_after_post", true)
		}
		if c.Tool != core.ToolSpawn {
			continue
		}
		sub, ok := subs[t.ids[i]]
		if !ok {
			continue
		}
		delete(subs, t.ids[i]) // a sub-agent is attached once
		a := attachSubagents(sub, subs, early, late)
		agent.Calls[i].Sub = &a
	}
	return agent
}

// wireInputs are the inputs of the calls the main agent made as the model sent them, by call id, as
