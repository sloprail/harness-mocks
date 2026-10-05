package replay

import (
	"fmt"
	"math"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// modelTurns are the calls the model made in one recorded rollout, in order,
// and its final answer. Codex's model calls its tools from a JS `exec` call
// (tools.exec_command({...}), tools.multi_agent_v1__spawn_agent({...})), so the
// calls are read out of that JS by parsing and evaluating it (js_run.go); codex's own
// tool names are mapped to the unified ones here. A call that only looks around
// (ALL_TOOLS) makes none. What the adapter cannot map is an error, never a guess.
func modelTurns(records []map[string]any, receipts []string) (agent core.Agent, err error) {
	var calls []core.Call
	var said *string
	js := newJSRun()
	var told []string        // the ids of the sub-agents the model was told of (spawn answers), in order
	var spawns []int         // the numbers of the spawn calls among the rollout's calls, in order
	derived := map[any]int{} // how many tool calls each script (by call id) was read to make, until its output is seen
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
				calls = append(calls, core.Call{Tool: core.ToolAnswer, Input: map[string]any{"text": text}})
				said = nil
			} else if said != nil {
				return core.Agent{}, fmt.Errorf("two commentary messages before one call: the first would be lost")
			} else {
				said = &text
			}
		case p["type"] == "custom_tool_call_output":
			told = append(told, agentIDs(p["output"])...)
			if err := checkOutput(p, derived); err != nil {
				return core.Agent{}, err
			}
			delete(derived, p["call_id"])
		case p["type"] == "custom_tool_call":
			src, _ := p["input"].(string)
			made, err := js.script(src)
			if err != nil {
				return core.Agent{}, err
			}
			derived[p["call_id"]] = len(made)
			for _, m := range made {
				c, err := unify(m, spawns, told)
				if err != nil {
					return core.Agent{}, err
				}
				if c.Tool == core.ToolSpawn {
					spawns = append(spawns, m.Num)
				}
				c.Said, said = said, nil
				calls = append(calls, c)
			}
		}
	}
	if len(derived) > 0 {
		return core.Agent{}, fmt.Errorf("a script of the model has no recorded output: its calls may not have run")
	}
	if err := attachReceipts(calls, receipts); err != nil {
		return core.Agent{}, err
	}
	final := ""
	if n := len(calls); n > 0 && calls[n-1].Tool == core.ToolAnswer { // the last answer is the final one
		final, _ = calls[n-1].Input["text"].(string)
		calls = calls[:n-1]
	}
	return core.Agent{Calls: calls, Final: final}, nil
}

// unify is the unified call of one of codex's tool calls.
func unify(m jsCall, spawns []int, told []string) (core.Call, error) {
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
		cmd, ok := arg["cmd"].(string)
		if !ok {
			return core.Call{}, fmt.Errorf("an exec_command whose cmd is not a string")
		}
		in := map[string]any{"command": cmd}
		for k, v := range arg { // the harness's other options go to the mock as given; what the mock does with one is the mock's own (today it ignores them, which matters only for a workdir other than the run's directory, refused below)
			if k == "cmd" || k == "yield_time_ms" {
				continue
			}
			if k == "workdir" && v != "<RUN>" {
				return core.Call{}, fmt.Errorf("an exec_command run in %v: the mock runs in the run's directory only", v)
			}
			sv, ok := scalar(v)
			if !ok {
				return core.Call{}, fmt.Errorf("an exec_command whose %s is not a string, number or boolean", k)
			}
			in[k] = sv
		}
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
		if len(arg) == 0 { // a call with no arguments, which the harness refuses (recorded: runs/agent-input-validation)
			return core.Call{Tool: core.ToolSpawn, Input: map[string]any{}}, nil
		}
		msg, ok := arg["message"].(string)
		if !ok {
			return core.Call{}, fmt.Errorf("a spawn_agent whose message is not a string")
		}
		in := map[string]any{"message": msg}
		for k, v := range arg {
			if k == "message" {
				continue
			}
			sv, ok := scalar(v)
			if !ok {
				return core.Call{}, fmt.Errorf("a spawn_agent whose %s is not a string, number or boolean", k)
			}
			in[k] = sv
		}
		return core.Call{Tool: core.ToolSpawn, Input: in}, nil
	case "multi_agent_v1__wait_agent":
		return unifyWait(arg, spawns, told)
	}
	return core.Call{}, fmt.Errorf("the model called tools.%s: the adapter maps exec_command, spawn_agent and wait_agent", m.Name)
}
