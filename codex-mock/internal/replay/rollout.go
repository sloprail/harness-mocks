package replay

import (
	"fmt"
	"math"
	"slices"
	"strings"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// modelTurns are the calls the model made in one recorded rollout, in order,
// and its final answer. Codex's model calls its tools from a JS `exec` call
// (tools.exec_command({...}), tools.multi_agent_v1__spawn_agent({...})), so the
// calls are read out of that JS by parsing and evaluating it (js_run.go); codex's own
// tool names are mapped to the unified ones here. A call that only looks around
// (ALL_TOOLS) makes none. What the adapter cannot map is an error, never a guess.
func modelTurns(records []map[string]any) (agent core.Agent, err error) {
	var calls []core.Call
	var final string
	var said *string
	js := newJSRun()
	for _, rec := range records {
		p, _ := rec["payload"].(map[string]any)
		if rec["type"] != "response_item" || p == nil {
			continue
		}
		switch {
		case p["type"] == "function_call" || p["type"] == "custom_tool_call" && p["name"] != "exec":
			return core.Agent{}, fmt.Errorf("the model called %v: the adapter maps only exec", p["name"])
		case p["type"] == "message" && p["role"] == "assistant":
			text := ""
			for _, c := range p["content"].([]any) {
				text += c.(map[string]any)["text"].(string)
			}
			if p["phase"] == "final_answer" {
				final = text
			} else {
				said = &text
			}
		case p["type"] == "custom_tool_call":
			src, _ := p["input"].(string)
			made, err := js.script(src)
			if err != nil {
				return core.Agent{}, err
			}
			for _, m := range made {
				if m.Name == "multi_agent_v1__wait_agent" {
					continue // not a call of the replay: the mock's spawn_agent waits for its sub-agent itself
				}
				c, err := unify(m)
				if err != nil {
					return core.Agent{}, err
				}
				c.Said, said = said, nil
				calls = append(calls, c)
			}
		}
	}
	return core.Agent{Calls: calls, Final: final}, nil
}

// unify is the unified call of one of codex's tool calls.
func unify(m jsCall) (core.Call, error) {
	var arg map[string]any
	if len(m.Args) != 1 {
		return core.Call{}, fmt.Errorf("the model called tools.%s with %d arguments: the adapter maps one", m.Name, len(m.Args))
	}
	arg, isObject := m.Args[0].(map[string]any)
	if !isObject {
		return core.Call{}, fmt.Errorf("the model called tools.%s with something other than an object: the adapter maps only an object", m.Name)
	}
	switch m.Name {
	case "exec_command":
		if err := knownKeys(arg, "cmd", "yield_time_ms", execCarried); err != nil {
			return core.Call{}, err
		}
		cmd, ok := arg["cmd"].(string)
		if !ok {
			return core.Call{}, fmt.Errorf("an exec_command whose cmd is not a string")
		}
		in := map[string]any{"command": cmd}
		if y, present := arg["yield_time_ms"]; present {
			n, ok := y.(number)
			if !ok {
				return core.Call{}, fmt.Errorf("an exec_command whose yield_time_ms is not a number")
			}
			if n.f != math.Trunc(n.f) || math.Abs(n.f) > 1<<31 {
				return core.Call{}, fmt.Errorf("an exec_command whose yield_time_ms is not a whole number of milliseconds")
			}
			in["yield_time_ms"] = int(n.f)
		}
		return core.Call{Tool: core.ToolShell, Input: in}, nil
	case "multi_agent_v1__spawn_agent":
		if err := knownKeys(arg, "message", "", ""); err != nil {
			return core.Call{}, err
		}
		if len(arg) == 0 { // a call with no arguments, which the harness refuses (recorded: runs/agent-input-validation)
			return core.Call{Tool: core.ToolSpawn, Input: map[string]any{}}, nil
		}
		msg, ok := arg["message"].(string)
		if !ok {
			return core.Call{}, fmt.Errorf("a spawn_agent whose message is not a string")
		}
		return core.Call{Tool: core.ToolSpawn, Input: map[string]any{"message": msg}}, nil
	}
	return core.Call{}, fmt.Errorf("the model called tools.%s: the adapter maps exec_command, spawn_agent and (as nothing) wait_agent", m.Name)
}

// execCarried are the exec_command arguments the replay does not hand to the
// mock: the mock runs in the run's directory with its own shell, and caps no output.
// A recording where one of them mattered replays red, since the whole output is compared.
const execCarried = "workdir max_output_tokens shell login tty"

// knownKeys refuses a call that has an argument the adapter neither maps (a, b) nor
// names as carried (the space-separated names in carried): a dropped argument would
// make the replay a different call.
func knownKeys(arg map[string]any, a, b, carried string) error {
	for k := range arg {
		if k != a && k != b && !slices.Contains(strings.Fields(carried), k) {
			return fmt.Errorf("a call with an argument %q, which the adapter does not map", k)
		}
	}
	return nil
}
