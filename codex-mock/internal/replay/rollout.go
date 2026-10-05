package replay

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

var (
	reTool  = regexp.MustCompile(`tools\.(\w+)\(`)
	reExec  = regexp.MustCompile(`(?s)tools\.exec_command\(\{(.*?)\}\)`)
	reSpawn = regexp.MustCompile(`(?s)tools\.multi_agent_v1__spawn_agent\(\{message:\s*("(?:[^"\\]|\\.)*")`)
	reYield = regexp.MustCompile(`yield_time_ms:\s*(\d+)`)
	reCmd   = regexp.MustCompile(`cmd:\s*("(?:[^"\\]|\\.)*")`)
)

// modelTurns are the calls the model made in one recorded rollout, in order,
// and its final answer. Codex's model calls its tools from a JS `exec` call
// (tools.exec_command({...}), tools.multi_agent_v1__spawn_agent({...})), so
// the calls are read out of that JS (codex's own tool names are mapped to the
// unified ones here); a call that only looks around (ALL_TOOLS)
// makes none. What the adapter cannot map is an error, never a guess.
func modelTurns(records []map[string]any) (agent core.Agent, err error) {
	var calls []core.Call
	var final string
	var said *string
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
			js, _ := p["input"].(string)
			for _, m := range reTool.FindAllStringSubmatch(js, -1) {
				switch m[1] {
				case "exec_command", "multi_agent_v1__spawn_agent", "multi_agent_v1__wait_agent":
				default:
					return core.Agent{}, fmt.Errorf("the model called tools.%s: the adapter maps exec_command and spawn_agent", m[1])
				}
			}
			for _, m := range reExec.FindAllStringSubmatch(js, -1) {
				var cmd string
				c := reCmd.FindStringSubmatch(m[1])
				if c == nil || json.Unmarshal([]byte(c[1]), &cmd) != nil {
					return core.Agent{}, fmt.Errorf("an exec_command whose cmd is not a string literal")
				}
				in := map[string]any{"command": cmd}
				if y := reYield.FindStringSubmatch(m[1]); y != nil {
					n, _ := strconv.Atoi(y[1])
					in["yield_time_ms"] = n
				}
				calls = append(calls, core.Call{Said: said, Tool: core.ToolShell, Input: in})
				said = nil
			}
			for _, m := range reSpawn.FindAllStringSubmatch(js, -1) {
				var msg string
				if json.Unmarshal([]byte(m[1]), &msg) != nil {
					return core.Agent{}, fmt.Errorf("a spawn_agent whose message is not a string literal")
				}
				calls = append(calls, core.Call{Said: said, Tool: core.ToolSpawn, Input: map[string]any{"message": msg}})
				said = nil
			}
		}
	}
	return core.Agent{Calls: calls, Final: final}, nil
}
