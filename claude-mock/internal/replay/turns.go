package replay

import (
	"fmt"

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
	var before []string
	trigger := "manual" // of the compaction whose summary comes next: its boundary says
	preserved := 0      // and how many of the last messages it kept
	logical := ""       // and whether its logical parent was a record the file holds
	segment := true     // and whether it names a preserved segment
	written := map[string]bool{}
	for _, r := range records {
		if id, _ := r["uuid"].(string); id != "" {
			written[id] = true
		}
	}
	for _, rec := range records {
		if endsTurn(rec) && said != nil { // a new turn begins: what was said last was the answer that ended the one before
			t.agent.Calls = append(t.agent.Calls, core.Call{Tool: core.ToolAnswer, Input: map[string]any{"text": *said}, SaidBefore: before})
			t.ids = append(t.ids, "")
			said, before = nil, nil
		}
		if c, ok := compaction(rec, trigger, preserved, logical, segment); ok {
			t.agent.Calls = append(t.agent.Calls, c)
			t.ids = append(t.ids, "")
		}
		if meta, _ := rec["compactMetadata"].(map[string]any); rec["subtype"] == "compact_boundary" && meta != nil {
			trigger, _ = meta["trigger"].(string)
			preserved, logical = 0, ""
			_, segment = meta["preservedSegment"]
			if lp, _ := rec["logicalParentUuid"].(string); lp != "" && !written[lp] {
				logical = "unwritten"
			}
			if pm, _ := meta["preservedMessages"].(map[string]any); pm != nil {
				uuids, _ := pm["uuids"].([]any)
				preserved = len(uuids)
			}
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
				said = &text
			case "tool_use":
				call, err := unify(block)
				if err != nil {
					return turns{}, err
				}
				call.Said, call.SaidBefore = said, before
				said, before = nil, nil
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

// toolPrefix starts the unified name of a tool the claude adapter passes on as it is: the mock either
// carries it out or refuses it, and what it does with it is then the replay's finding.
const toolPrefix = "claude:"

// unify maps a tool_use block onto the unified vocabulary: Bash is a shell command, Agent (alias
// Task) a spawn (its prompt is the message); any other tool goes on under its own name.
func unify(block map[string]any) (core.Call, error) {
	name, _ := block["name"].(string)
	input, _ := block["input"].(map[string]any)
	in := make(map[string]any, len(input)+1)
	for k, v := range input {
		in[k] = v
	}
	switch name {
	case "Bash":
		return core.Call{Tool: core.ToolShell, Input: in}, nil
	case "Agent", "Task":
		prompt, _ := input["prompt"].(string)
		in["message"] = prompt
		return core.Call{Tool: core.ToolSpawn, Input: in}, nil
	case "":
		return core.Call{}, fmt.Errorf("a tool call with no name")
	}
	return core.Call{Tool: toolPrefix + name, Input: in}, nil
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

// compaction is the compaction a record is the summary of: the user record the harness writes with
// isCompactSummary, whose text is what the agent is given in place of what was compacted.
func compaction(rec map[string]any, trigger string, preserved int, logical string, segment bool) (core.Call, bool) {
	if rec["type"] != "user" || rec["isCompactSummary"] != true {
		return core.Call{}, false
	}
	msg, _ := rec["message"].(map[string]any)
	text, ok := msg["content"].(string)
	in := map[string]any{"summary": text, "trigger": trigger}
	if preserved > 0 {
		in["preserve"] = preserved
	}
	if logical != "" {
		in["logical_parent"] = logical
	}
	if !segment {
		in["preserved_segment"] = false
	}
	return core.Call{Tool: core.ToolCompact, Input: in}, ok
}
