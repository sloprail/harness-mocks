package replay

import (
	"fmt"
	"time"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// turns are what an agent did, with the id each of its calls had, so that the
// agent a call started can be attached to it.
type turns struct {
	agent core.Agent
	ids   []string // the id of each of agent.Calls
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
				said, before = nil, nil
				id, _ := block["id"].(string)
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

// unify maps a tool_use block onto the unified vocabulary: Bash is a shell
// command, Agent a spawn (its prompt is the message). Any other tool is not
// mapped yet.
func unify(block map[string]any) (core.Call, error) {
	name, _ := block["name"].(string)
	input, _ := block["input"].(map[string]any)
	in := make(map[string]any, len(input)+1)
	for k, v := range input {
		in[k] = v
	}
	if tool, ok := fileTools[name]; ok {
		return core.Call{Tool: tool, Input: in}, nil
	}
	switch name {
	case "Bash":
		return core.Call{Tool: core.ToolShell, Input: in}, nil
	case "Agent", "Task":
		prompt, _ := input["prompt"].(string)
		in["message"] = prompt
		return core.Call{Tool: core.ToolSpawn, Input: in}, nil
	}
	if name == "" {
		return core.Call{}, fmt.Errorf("a tool call with no name")
	}
	return core.Call{Tool: toolPrefix + name, Input: in}, nil // any other tool goes on under its own name: the mock runs or refuses it
}

// attachSubagents is the main agent's calls with each spawn's sub-agent attached,
// and theirs in turn, found by the id of the call that started them. A spawn
// with no recorded sub-agent keeps none: the call itself was refused.
func attachSubagents(t turns, subs map[string]turns) core.Agent {
	agent := core.Agent{Calls: append([]core.Call(nil), t.agent.Calls...), Final: t.agent.Final, FinalAt: t.agent.FinalAt}
	for i, c := range agent.Calls {
		if c.Tool != core.ToolSpawn {
			continue
		}
		sub, ok := subs[t.ids[i]]
		if !ok {
			continue
		}
		delete(subs, t.ids[i]) // a sub-agent is attached once
		a := attachSubagents(sub, subs)
		agent.Calls[i].Sub = &a
	}
	return agent
}

// wireInputs are the inputs of the calls the main agent made as the model sent them, by call id, as

// toolPrefix starts the unified name of a tool the claude adapter passes on as it is.
const toolPrefix = "claude:"

// endsTurn is whether a record opens a turn of the model's own accord: a user record whose content is
// text (a task's notification, a hook's feedback), not a tool's result.
func endsTurn(rec map[string]any) bool {
	if rec["type"] != "user" {
		return false
	}
	msg, _ := rec["message"].(map[string]any)
	_, text := msg["content"].(string)
	return text
}
