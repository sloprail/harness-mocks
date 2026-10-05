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
func modelTurns(records []map[string]any) (core.Agent, error) {
	var agent core.Agent
	var said *string
	var lookup map[string]any // a GetDynamicTools of one named tool, which the call that follows needs
	flush := func() {         // an answer: the text no call followed
		if said != nil {
			agent.Calls = append(agent.Calls, core.Call{Tool: core.ToolAnswer, Input: map[string]any{"text": *said}})
			said = nil
		}
	}
	for _, rec := range records {
		switch rec["role"] {
		case "user":
			flush()
		case "assistant":
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
						lookup = lookupOf(block)
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
				said = nil
				calls[0].Said = text
				agent.Calls = append(agent.Calls, calls...)
			case text != nil:
				flush()
				said = text
			}
		}
	}
	if said != nil {
		agent.Final = *said
	}
	return agent, nil
}

// unify maps a tool_use block onto the unified vocabulary: Shell is a shell
// command, Task a spawn (its prompt is the message), Read and Write file reads
// and writes. Any other tool is not mapped: the mock has none of them.
func unify(block map[string]any) (core.Call, error) {
	name, _ := block["name"].(string)
	input, _ := block["input"].(map[string]any)
	in := make(map[string]any, len(input)+1)
	for k, v := range input {
		in[k] = v
	}
	switch name {
	case "Grep":
		return core.Call{Tool: core.ToolSearchFiles, Input: in}, nil
	case "Delete":
		return core.Call{Tool: core.ToolDeleteFile, Input: in}, nil
	case "CallDynamicTool":
		args, _ := input["arguments"].(map[string]any)
		return core.Call{Tool: core.ToolMCP, Input: map[string]any{"server": input["namespace"], "tool": input["toolName"], "arguments": args}}, nil
	case "Shell":
		return core.Call{Tool: core.ToolShell, Input: in}, nil
	case "Task":
		prompt, _ := input["prompt"].(string)
		in["message"] = prompt
		return core.Call{Tool: core.ToolSpawn, Input: in}, nil
	case "Read":
		return core.Call{Tool: core.ToolReadFile, Input: in}, nil
	case "Write":
		in["content"] = in["contents"]
		delete(in, "contents")
		return core.Call{Tool: core.ToolWriteFile, Input: in}, nil
	}
	return core.Call{}, fmt.Errorf("the model called %s: the mock has no such tool", name)
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
