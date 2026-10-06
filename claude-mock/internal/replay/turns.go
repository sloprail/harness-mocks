package replay

import (
	"fmt"
	"strings"
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
	records = withoutForkContext(records)
	done := map[string]time.Time{} // when each call's result was given back
	for _, rec := range records {
		if rec["type"] == "user" {
			for id := range resultIDs(rec) {
				done[id] = stampOf(rec)
			}
		}
		if rec["type"] == "user" && isStopFeedback(rec) {
			// a Stop hook blocked the end of the turn: what the model said before it was a reply of its
			// own, and the model answers again
			if said == nil {
				return turns{}, fmt.Errorf("a Stop hook's feedback came to a model that had said nothing")
			}
			t.agent.Calls = append(t.agent.Calls, core.Call{Tool: toolReply, At: saidAt, Input: map[string]any{"text": *said}})
			t.ids = append(t.ids, "")
			said = nil
			continue
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
					return turns{}, fmt.Errorf("the model said two things before one call: the adapter keeps one")
				}
				text, _ := block["text"].(string)
				said, saidAt = &text, stampOf(rec)
			case "tool_use":
				call, err := unify(block)
				if err != nil {
					return turns{}, err
				}
				call.Said, call.At = said, stampOf(rec)
				said = nil
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
	return core.Call{}, fmt.Errorf("the model called %s: the adapter maps Bash, Read, Write, Edit, Glob and Agent", name)
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

// nudge starts the user record the harness leaves when a model's response had no visible output.
const nudge = "[Your previous response had no visible output."

// isNudge is whether a user record is that nudge.
func isNudge(rec map[string]any) bool {
	msg, _ := rec["message"].(map[string]any)
	s, _ := msg["content"].(string)
	return strings.HasPrefix(s, nudge)
}
