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
	var saidThought *core.Thinking          // the thought of the text-only response said holds
	var pending, saidCompact map[string]any // a compaction the harness made, not yet put on the response that follows it
	ri, lookupRI := -1, -1                  // the response that looked a tool up
	var lookup map[string]any               // a GetDynamicTools of one named tool, which the call that follows needs
	flush := func() {                       // an answer: the text no call followed
		if said != nil {
			agent.Calls = append(agent.Calls, core.Call{Tool: core.ToolAnswer, Input: map[string]any{"text": *said}, Thinking: saidThought, Compact: saidCompact})
			said, saidThought, saidCompact = nil, nil, nil
		}
	}
	for _, rec := range records {
		switch rec["role"] {
		case "user":
			flush()
		case compactionMarker:
			pending, _ = rec["fields"].(map[string]any)
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
				if pending != nil && saidCompact != nil {
					return core.Agent{}, fmt.Errorf("the harness compacted twice before one response")
				}
				calls[0].Compact = pending
				if calls[0].Compact == nil {
					calls[0].Compact = saidCompact
				}
				pending, said, saidThought, saidCompact = nil, nil, nil, nil
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
	if pending != nil || saidCompact != nil {
		return core.Agent{}, fmt.Errorf("the harness compacted before the final answer: the compaction is not put on an answer")
	}
	if said != nil {
		agent.Final, agent.FinalThinking = *said, saidThought
	}
	return agent, nil
}
