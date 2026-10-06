package replay

import (
	"fmt"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// modelTurns are what the model did in one recorded transcript, as an agent.
// A transcript holds one record per model response: its text and its tool
// calls together, then a user record when the harness gives the agent a turn
// (the prompt, or what follows a finished background task). Only the assistant
// records are the model's, so only they are read; the user records are the
// harness's, which the mock makes itself.
//
// A response with calls is those calls, the first with the text the model said
// before it, the rest in the same turn. A response of text alone that the next
// response's calls follow, with no user record between, is said before them;
// otherwise it is an answer, and the last answer is the agent's final one. The
// tools are mapped onto the unified ones here; what the adapter cannot map is an
// error, never a guess.
//
// thoughts are what the model thought, by the number of the response that had
// it: a response with no thought is not in them (recorded: runs/symlinked-cwd,
// the first response thought and the second did not).
func modelTurns(records []map[string]any, heard []thoughtAt) (core.Agent, error) {
	var agent core.Agent
	thoughts, err := placeThoughts(records, heard)
	if err != nil {
		return core.Agent{}, err
	}
	thought := func(i int) *core.Thinking { return thoughts[i] }
	var said *string
	var saidThought *core.Thinking // the thought of the text-only response said holds
	ri, lookupRI := -1, -1         // the response that looked a tool up
	var lookup map[string]any      // a GetDynamicTools of one named tool, which the call that follows needs
	flush := func() {              // an answer: the text no call followed
		if said != nil {
			agent.Calls = append(agent.Calls, core.Call{Tool: core.ToolAnswer, Input: map[string]any{"text": *said}, Thinking: saidThought})
			said, saidThought = nil, nil
		}
	}
	for _, rec := range records {
		switch rec["role"] {
		case "user":
			flush()
		case "assistant":
			ri++
			var calls []core.Call
			var text *string
			msg, _ := rec["message"].(map[string]any)
			blocks, _ := msg["content"].([]any)
			for _, b := range blocks {
				block, _ := b.(map[string]any)
				switch block["type"] {
				case "text":
					if text != nil {
						return core.Agent{}, fmt.Errorf("a response of the model holds two texts: the adapter keeps one")
					}
					t, _ := block["text"].(string)
					text = &t
				case "tool_use":
					if lookupOf(block) != nil {
						lookup, lookupRI = lookupOf(block), ri
						continue
					}
					c, err := unify(block)
					if err != nil {
						return core.Agent{}, err
					}
					if c.Tool == core.ToolMCP && !sameTool(lookup, c.Input) {
						return core.Agent{}, fmt.Errorf("the model called CallDynamicTool without looking the tool up first: the mock looks it up itself, once")
					}
					lookup = nil
					c.SameTurn = len(calls) > 0
					calls = append(calls, c)
				default:
					return core.Agent{}, fmt.Errorf("the transcript holds a %v block: the adapter reads text and tool_use", block["type"])
				}
			}
			switch {
			case len(calls) > 0:
				if said != nil && text != nil {
					return core.Agent{}, fmt.Errorf("the model said two things before one call: the adapter keeps one")
				}
				if text == nil {
					text = said
				}
				if saidThought != nil && thought(ri) != nil {
					return core.Agent{}, fmt.Errorf("the model thought in two responses before one step: the mock plays them as one")
				}
				calls[0].Thinking = thought(ri)
				if calls[0].Thinking == nil {
					calls[0].Thinking = saidThought
				}
				if lookupRI >= 0 && lookupRI != ri && thought(lookupRI) != nil { // the lookup is the mock's own: its thought joins the call's
					if calls[0].Thinking != nil {
						return core.Agent{}, fmt.Errorf("the model thought in the response that looked a tool up and in the one that called it: the mock plays them as one")
					}
					calls[0].Thinking = thought(lookupRI)
				}
				lookupRI = -1
				said, saidThought = nil, nil
				calls[0].Said = text
				agent.Calls = append(agent.Calls, calls...)
			case text != nil && said != nil:
				// a second text-only response with no user turn between is one more thing
				// the model said in the same turn (it ended with its end-of-stream token, recorded:
				// runs/nested-subagents-background): the stream joins what was said, so it is one
				// answer here too
				if saidThought != nil && thought(ri) != nil {
					return core.Agent{}, fmt.Errorf("the model thought in two text responses of one turn: the mock plays them as one")
				}
				joined := *said + *text
				said = &joined
				if saidThought == nil {
					saidThought = thought(ri)
				}
			case text != nil:
				said, saidThought = text, thought(ri)
			}
		}
	}
	if said != nil {
		agent.Final, agent.FinalThinking = *said, saidThought
	}
	return agent, nil
}

// lookupOf is what a GetDynamicTools block asks for when it names one tool of
// one server (namespace and toolName), which the mock does on its own before it
// calls an MCP tool; nil for any other block, GetDynamicTools searching by a
// pattern included, which the mock has no such tool for.
func lookupOf(block map[string]any) map[string]any {
	if name, _ := block["name"].(string); name != "GetDynamicTools" {
		return nil
	}
	input, _ := block["input"].(map[string]any)
	if input["namespace"] == nil || input["toolName"] == nil {
		return nil
	}
	return input
}

// sameTool reports whether the lookup named the tool the call is of.
func sameTool(lookup, call map[string]any) bool {
	return lookup != nil && lookup["namespace"] == call["server"] && lookup["toolName"] == call["tool"]
}

// placeThoughts gives each thought the index of the response, among all the
// transcript's, that had it. A request is the responses between two user
// records (the first user record is the prompt), and the requests a conversation
// thought in are taken in the order of the requests themselves.
func placeThoughts(records []map[string]any, heard []thoughtAt) (map[int]*core.Thinking, error) {
	var starts, sizes []int // the first response of each request, and how many it has
	responses, fresh := 0, true
	for _, rec := range records {
		switch rec["role"] {
		case "user":
			fresh = true
		case "assistant":
			if fresh {
				starts, sizes, fresh = append(starts, responses), append(sizes, 0), false
			}
			sizes[len(sizes)-1]++
			responses++
		}
	}
	out := map[int]*core.Thinking{}
	maxRequest := -1
	for _, h := range heard {
		maxRequest = max(maxRequest, h.request)
	}
	if maxRequest >= len(starts) {
		return nil, fmt.Errorf("a thought names request %d of a conversation of %d", maxRequest, len(starts))
	}
	if maxRequest >= 0 && len(starts) > 1 && maxRequest+1 != len(starts) {
		return nil, fmt.Errorf("the model thought in %d of a conversation's %d requests: which request a thought belongs to is not told", maxRequest+1, len(starts))
	}
	for _, h := range heard {
		if h.response < 0 || h.response >= sizes[h.request] {
			return nil, fmt.Errorf("a thought names response %d of a request of %d", h.response, sizes[h.request])
		}
		out[starts[h.request]+h.response] = h.thinking
	}
	return out, nil
}
