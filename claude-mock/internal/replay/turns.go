package replay

import (
	"fmt"
	"strings"

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
	for _, rec := range records {
		if rec["type"] == "user" && isStopFeedback(rec) {
			// a Stop hook blocked the end of the turn: what the model said before it was a reply of its
			// own, and the model answers again
			if said == nil {
				return turns{}, fmt.Errorf("a Stop hook's feedback came to a model that had said nothing")
			}
			t.agent.Calls = append(t.agent.Calls, core.Call{Tool: toolReply, Input: map[string]any{"text": *said}})
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
					return turns{}, fmt.Errorf("the model said two things before one call: the adapter keeps one")
				}
				text, _ := block["text"].(string)
				said = &text
			case "tool_use":
				call, err := unify(block)
				if err != nil {
					return turns{}, err
				}
				call.Said = said
				said = nil
				id, _ := block["id"].(string)
				t.agent.Calls = append(t.agent.Calls, call)
				t.ids = append(t.ids, id)
			}
		}
	}
	if said != nil {
		t.agent.Final = *said
	}
	return t, nil
}

// toolReply is the adapter's unified name of an answer the model gave that a Stop hook then refused
// to end the turn on: Input "text". It is not a tool call: the model said it, and was told to go on.
const toolReply = "reply"

// stopFeedback starts the user record a blocking Stop hook leaves.
const stopFeedback = "Stop hook feedback:"

// isStopFeedback is whether a user record is a Stop hook's feedback.
func isStopFeedback(rec map[string]any) bool {
	msg, _ := rec["message"].(map[string]any)
	s, _ := msg["content"].(string)
	return strings.HasPrefix(s, stopFeedback)
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
	agent := core.Agent{Calls: append([]core.Call(nil), t.agent.Calls...), Final: t.agent.Final}
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
