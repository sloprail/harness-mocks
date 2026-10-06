package replay

import (
	"fmt"
	"strings"

	core "github.com/sloprail/harness-mocks/internal/replay"
	"github.com/sloprail/harness-mocks/internal/scenario"
)

// mockCall is the mock's name for a unified call; the input is copied, as the
// spawn's script parameter is added to it.
func mockCall(c core.Call) modelCall {
	if c.Tool == core.ToolAnswer {
		text, _ := c.Input["text"].(string)
		return modelCall{Final: &text}
	}
	name := c.Tool
	if c.Tool == core.ToolPatch { // the mock's apply_patch takes the patch as its command
		return modelCall{Text: c.Said, Name: "apply_patch", Input: map[string]any{"command": strings.ReplaceAll(fmt.Sprint(c.Input["patch"]), "<RUN>", runPlaceholder)}, More: c.More}
	}
	if c.Tool == core.ToolPoll { // the session is found again by position in the script
		in := map[string]any{"session_id": map[string]any{"session": c.Input["session"]}}
		for _, k := range []string{"yield_time_ms", "max_output_tokens"} {
			if v, ok := c.Input[k]; ok {
				in[k] = v
			}
		}
		return modelCall{Text: c.Said, Name: "write_stdin", Input: in, More: c.More}
	}
	if c.Tool == core.ToolCompact {
		return modelCall{Name: core.ToolCompact, Input: map[string]any{"trigger": c.Input["trigger"]}}
	}
	if c.Tool == core.ToolShell {
		name = "Bash"
	}
	in := make(map[string]any, len(c.Input)+1)
	for k, v := range c.Input {
		if s, ok := v.(string); ok { // the run's own directory: the recording has it as <RUN>
			v = strings.ReplaceAll(s, "<RUN>", runPlaceholder)
		}
		in[k] = v
	}
	if c.Tool == core.ToolWait { // the targets are the receipts of the agent's spawns: the script puts each one in, by its position
		var ids []any
		for _, k := range c.Input["targets"].([]int) {
			ids = append(ids, map[string]any{"spawned": k})
		}
		in["targets"] = ids
	}
	return modelCall{Text: c.Said, Name: name, Input: in, More: c.More}
}

// agentScript is an agent's calls as the mock's steps, with the gate of its final answer.
type agentScript struct {
	calls []modelCall
	final scenario.Gate
}

// agentCalls are an agent's calls as the mock's steps. Each sub-agent it starts, and each of theirs,
// becomes a script file of its own in scripts, named by the order the whole tree's spawns are met in
// (n counts them), and its spawn call names it.
func agentCalls(a core.Agent, parent *core.Agent, scripts map[string]string, n *int) agentScript {
	calls := make([]modelCall, len(a.Calls))
	gates := gatesOf(a, parent)
	for i, c := range a.Calls {
		calls[i] = mockCall(c)
		calls[i].Gate = gates[i]
		if c.Tool != core.ToolSpawn || c.Sub == nil {
			continue
		}
		k := *n
		*n++
		sub := agentCalls(*c.Sub, &a, scripts, n)
		name := fmt.Sprintf("sub%d.sh", k)
		scripts[name] = scriptFor(fmt.Sprintf("sub%d", k), 0, sub.calls, c.Sub.Final, c.Sub.Unfinished, sub.final)
		calls[i].Input["script"] = scriptsDir + "/" + name
	}
	return agentScript{calls, gates[len(calls)]}
}
