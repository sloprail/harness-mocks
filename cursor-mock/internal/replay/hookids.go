package replay

import (
	"strings"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// hookIDKey is the key of a unified call's input that holds the id its hooks name
// it by, when that is not the call's own.
const hookIDKey = "hook_id"

// framedCalls are the ids of the tool calls the main session's stream shows, in
// the order their first frame appears (a call refused before it starts has only a
// completed frame): the calls the model made, the lookup of an MCP tool (which the
// mock makes itself) left out.
func framedCalls(stream []map[string]any, session string) []string {
	var ids []string
	seen := map[string]bool{}
	for _, f := range stream {
		if f["type"] != "tool_call" || f["session_id"] != session {
			continue
		}
		call, _ := f["tool_call"].(map[string]any)
		if _, lookup := call["getMcpToolsToolCall"]; lookup {
			continue
		}
		if id, _ := f["call_id"].(string); !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}

// hookedCalls are the ids the main session's hooks name its calls by, each once,
// in the order they first appear, with the tool_input of the first payload that
// carries one (the call's command, file or prompt as the hook saw it).
func hookedCalls(payloads []map[string]any, session string) (ids []string, inputs map[string]map[string]any) {
	inputs = map[string]map[string]any{}
	for _, p := range payloads {
		id, _ := p["tool_use_id"].(string)
		if name, _ := p["tool_name"].(string); id == "" || p["session_id"] != session || strings.HasPrefix(name, "MCP:") {
			continue // an MCP call's hooks always name it by an id of their own, which the mock makes itself
		}
		if _, seen := inputs[id]; !seen {
			ids = append(ids, id)
			inputs[id] = nil
		}
		if in, ok := p["tool_input"].(map[string]any); ok && inputs[id] == nil {
			inputs[id] = in
		}
	}
	return ids, inputs
}

// callMatches reports whether the hook's tool_input is that of the unified call:
// the command of a shell call, the prompt of a Task, the pattern of a Grep or the
// path of a file call, each equal to what the call was made with.
func callMatches(c core.Call, in map[string]any) bool {
	same := func(hook, call string) bool {
		h, _ := in[hook].(string)
		v, _ := c.Input[call].(string)
		return h != "" && h == v
	}
	switch c.Tool {
	case core.ToolShell:
		return same("command", "command")
	case core.ToolSpawn:
		return same("prompt", "message")
	case core.ToolSearchFiles:
		return same("pattern", "pattern")
	case core.ToolReadFile, core.ToolWriteFile, core.ToolDeleteFile:
		return same("file_path", "path")
	}
	return false
}

// nameHookIDs puts on each call of the main agent the id its hooks named it by,
// when the recording's hooks named some call by an id other than the call's own:
// Cursor did so in some runs and not in others. An id belongs to the first call
// not yet claimed whose input is the one the hook saw (the hook that named it
// carries a tool_input); an id whose call cannot be found that way, or a call a
// hook claims with a payload that holds no tool_input, is not replayed, as which
// id belongs to which call would be a guess.
func nameHookIDs(a *core.Agent, stream, payloads []map[string]any, session string) error {
	frames := framedCalls(stream, session)
	hooked, inputs := hookedCalls(payloads, session)
	own := map[string]bool{}
	for _, id := range frames {
		own[id] = true
	}
	differs := false
	for _, id := range hooked {
		differs = differs || !own[id]
	}
	if !differs {
		return nil
	}
	var calls []*core.Call
	for i := range a.Calls {
		if a.Calls[i].Tool != core.ToolAnswer {
			calls = append(calls, &a.Calls[i])
		}
	}
	claimed := make([]bool, len(calls))
	for _, id := range hooked {
		if own[id] { // named by the call's own id: nothing to tell
			continue
		}
		in := inputs[id]
		found := -1
		for i, c := range calls {
			if !claimed[i] && in != nil && callMatches(*c, in) {
				found = i
				break
			}
		}
		if found < 0 {
			return unbuildable("the hooks name a call by the id %q of its own, and no call of the run has the input the hook saw", id)
		}
		claimed[found] = true
		call := calls[found]
		withID := make(map[string]any, len(call.Input)+1)
		for k, v := range call.Input {
			withID[k] = v
		}
		withID[hookIDKey] = id
		call.Input = withID
	}
	return nil
}
