package replay

import (
	"encoding/json"
	"fmt"
	"strings"

	core "github.com/sloprail/harness-mocks/internal/replay"
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
}

// scriptsDir stands, in the scenario's calls, for the directory the sub-agents'
// scripts are written to.
const scriptsDir = "@SCRIPTS@"

// modelCall is one tool call the model made, in the mock's script vocabulary.
type modelCall struct {
	Text  *string        `json:"text,omitempty"` // what the model said just before the call, if it said anything
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
	// Final is the end of a turn: the model's answer, with no call.
	Final *string `json:"-"`
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
	calls := make([]modelCall, len(rec.Agent.Calls))
	n := 0
	for i, c := range rec.Agent.Calls {
		calls[i] = mockCall(c)
		if c.Tool == core.ToolSpawn && c.Sub != nil {
			name := fmt.Sprintf("sub%d.sh", n)
			subCalls := make([]modelCall, len(c.Sub.Calls))
			for j, sc := range c.Sub.Calls {
				subCalls[j] = mockCall(sc)
			}
			scripts[name] = scriptFor(fmt.Sprintf("sub%d", n), subCalls, c.Sub.Final)
			calls[i].Input["script"] = scriptsDir + "/" + name
			n++
		}
	}
	return Scenario{
		HooksJSON: rec.Setup["hooks.json"],
		Files:     files,
		Scripts:   scripts,
		Script:    scriptFor("main", calls, rec.Agent.Final),
		Prompt:    rec.Prompt,
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
	if c.Tool == core.ToolWait { // the targets are the receipts of the agent's spawns, which the script reads off its session
		var ids []string
		for _, k := range c.Input["targets"].([]int) {
			ids = append(ids, fmt.Sprintf("AGENT%d", k+1))
		}
		in["targets"] = ids
	}
	return modelCall{Text: c.Said, Name: name, Input: in}
}

// scriptFor is the mock script that plays the model's steps, a call or an
// answer each, in the order it took them. The step to play is the number of
// calls it has been answered (function_call_output records) plus the number of
// times a hook has sent it on (hook_prompt records), each of which spent an
// answer. The steps are inside the script, so a sub-agent's script is a file of
// its own. Call ids are unique across the run's scripts (tag), as the real ones
// are: the mock keys a still-running command by its call's id. Past the last
// step (an answer) the last step is played again.
func scriptFor(tag string, steps []modelCall, final string) string {
	steps = append(steps, modelCall{Final: &final})
	var lines []string
	for _, c := range steps {
		if c.Final != nil {
			b, _ := json.Marshal(map[string]any{"final": *c.Final})
			lines = append(lines, string(b))
			continue
		}
		content := []any{}
		if c.Text != nil {
			content = append(content, map[string]any{"type": "text", "text": *c.Text})
		}
		content = append(content, map[string]any{"type": "tool_use", "id": "IDPLACE", "name": c.Name, "input": c.Input})
		b, _ := json.Marshal(map[string]any{"type": "assistant", "message": map[string]any{"content": content}})
		lines = append(lines, string(b))
	}
	return fmt.Sprintf(`#!/bin/sh
n=$(grep -c function_call_output "$A10N_MOCK_SESSION_FILE")
k=$(grep -c '<hook_prompt' "$A10N_MOCK_SESSION_FILE")
steps=$(cat <<'STEPS_EOF'
%s
STEPS_EOF
)
step=$(printf '%%s\n' "$steps" | sed -n "$((n+k+1))p")
[ -n "$step" ] || step=$(printf '%%s\n' "$steps" | tail -1)
case "$step" in
'{"final":'*)
  text=$(printf '%%s' "$step" | jq -c .final)
  printf '%%s\n' "$(jq -nc --argjson t "$text" '{type:"assistant",message:{content:[{type:"text",text:$t}]}}')" "$(jq -nc --argjson t "$text" '{type:"result",subtype:"success",result:$t}')"
  ;;
*)
  for i in 1 2 3 4 5 6 7 8 9; do
    case "$step" in *AGENT$i*) step=$(printf '%%s' "$step" | sed "s/AGENT$i/$(jq -r 'select(.payload.type=="function_call_output")|.payload.output|try (fromjson|.agent_id) catch empty|select(.!=null)' "$A10N_MOCK_SESSION_FILE" | sed -n "${i}p")/g") ;; esac
  done
  printf '%%s\n' "$step" | sed "s/IDPLACE/call_%s_$n/"
  ;;
esac
`, strings.Join(lines, "\n"), tag)
}
