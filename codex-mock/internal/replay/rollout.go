package replay

import (
	"fmt"
	"time"

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
	sawFinal := false
	js := newJSRun()
	var told []string        // the ids of the sub-agents the model was told of (spawn answers), in order
	var spawns []int         // the numbers of the spawn calls among the rollout's calls, in order
	derived := map[any]int{} // how many tool calls each script (by call id) was read to make, until its output is seen
	ran := map[any][]int{}   // the calls each script made, by their place among the rollout's calls
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
				sawFinal = true
				calls = append(calls, core.Call{Tool: core.ToolAnswer, At: stampOf(rec), Input: map[string]any{"text": text}})
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
			for _, i := range ran[p["call_id"]] { // the script's calls finished when its output was recorded
				calls[i].Done = stampOf(rec)
			}
			delete(derived, p["call_id"])
		case p["type"] == "custom_tool_call":
			src, _ := p["input"].(string)
			made, err := js.script(src)
			if err != nil {
				return core.Agent{}, err
			}
			derived[p["call_id"]] = len(made)
			ran[p["call_id"]] = nil
			for i, m := range made {
				c, err := unify(m, spawns, told)
				if err != nil {
					return core.Agent{}, err
				}
				if c.Tool == core.ToolSpawn {
					spawns = append(spawns, m.Num)
				}
				c.Said, said = said, nil
				c.More, c.At = i+1 < len(made), stampOf(rec)
				ran[p["call_id"]] = append(ran[p["call_id"]], len(calls))
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
	unfinished := !sawFinal
	final, finalAt := "", time.Time{}
	if n := len(calls); n > 0 && calls[n-1].Tool == core.ToolAnswer { // the last answer is the final one
		final, finalAt = calls[n-1].Input["text"].(string), calls[n-1].At
		calls = calls[:n-1]
	}
	return core.Agent{Calls: calls, Final: final, FinalAt: finalAt, Unfinished: unfinished}, nil
}
