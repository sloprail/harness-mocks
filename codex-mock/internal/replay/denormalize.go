package replay

import (
	"fmt"
	"strings"

	core "github.com/sloprail/harness-mocks/internal/replay"
	"github.com/sloprail/harness-mocks/internal/scenario"
)

// Scenario is what the mock is given to replay a recording: the run's own
// setup (its hooks.json, its hook script, its prompt) and the scenario script
// that makes the model's calls, in the format the mock takes of any scenario.
type Scenario struct {
	HooksJSON string
	// Files are written into the repository (the hook script).
	Files map[string]string
	// Scripts are the sub-agents' scenario scripts. The real run's repository
	// holds none, so they sit beside it, at scriptsDir, which the run's own
	// directory replaces.
	Scripts map[string]string
	Script  string
	Prompt  string
	// Flags are run options the mock is given (--ephemeral, -c agents.max_depth=N, -C dir).
	Flags []string
	// ProjectHooksJSON is the project layer's hooks, written to <repo>/.codex/hooks.json.
	ProjectHooksJSON string
	// Interrupt is a run the user interrupted: the mock is sent SIGINT once its last command has started.
	Interrupt bool
	// Then are the later runs of the harness (a resume, a fork), each with its own script.
	Then []ThenStep
}

// ThenStep is a later run of the harness: its words, directory, prompt and script.
type ThenStep struct {
	Args   []string
	Cwd    string
	Prompt string
	Script string
}

// scriptsDir stands, in the scenario's calls, for the directory the sub-agents'
// scripts are written to.
const scriptsDir = "@SCRIPTS@"

// modelCall is one tool call the model made, in the mock's script vocabulary.
type modelCall struct {
	Text  *string        `json:"text,omitempty"` // what the model said just before the call, if it said anything
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
	// More is that another call of the model's script follows: it is not asked again after this call.
	More bool `json:"more,omitempty"`
	// Final is the end of a turn: the model's answer, with no call.
	Final *string `json:"-"`
	// Hang is an agent that never ends (an unfinished one).
	Hang bool `json:"-"`
	// Gate is what must have happened before the step is taken (gates.go).
	Gate scenario.Gate `json:"-"`
}

// runPlaceholder stands, in the scenario's calls, for the repository the mock runs
// in, which the replay's own directory replaces; the recording has it as <RUN>.
const runPlaceholder = "@RUN@"

// Denormalize turns the unified recording into the mock's scenario: the
// unified tools go back to the mock's names, and each sub-agent's turns become
// a script file of their own.
func Denormalize(rec core.Recording) Scenario {
	files := map[string]string{"hook.sh": rec.Setup["hook.sh"]}
	scripts := map[string]string{}
	main := agentCalls(rec.Agent, nil, scripts, new(int))
	return Scenario{
		HooksJSON:        rec.Setup["hooks.json"],
		Files:            files,
		Scripts:          scripts,
		Script:           scriptFor("main", 0, main.calls, rec.Agent.Final, false, main.final),
		Then:             thenScenario(rec),
		Prompt:           rec.Prompt,
		Flags:            strings.Fields(rec.Setup["flags"]),
		ProjectHooksJSON: rec.Setup["project-hooks.json"],
		Interrupt:        rec.Agent.Interrupted,
	}
}

// mockCall is the mock's name for a unified call; the input is copied, as the
// spawn's script parameter is added to it.
func mockCall(c core.Call) modelCall {
	if c.Tool == core.ToolAnswer {
		text, _ := c.Input["text"].(string)
		return modelCall{Final: &text}
	}
	name := c.Tool
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
