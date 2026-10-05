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
	return modelCall{Text: c.Said, Name: name, Input: in}
}

// scriptFor is the mock script that makes the given calls, one per model turn,
// then answers with final. The calls are inside it, so a sub-agent's script is
// a file of its own. Call ids are unique across the run's scripts (tag), as the
// real ones are: the mock keys a still-running command by its call's id.
func scriptFor(tag string, calls []modelCall, final string) string {
	var lines []string
	for _, c := range calls {
		content := []any{}
		if c.Text != nil {
			content = append(content, map[string]any{"type": "text", "text": *c.Text})
		}
		content = append(content, map[string]any{"type": "tool_use", "id": "IDPLACE", "name": c.Name, "input": c.Input})
		b, _ := json.Marshal(map[string]any{"type": "assistant", "message": map[string]any{"content": content}})
		lines = append(lines, string(b))
	}
	fin, _ := json.Marshal(final)
	return fmt.Sprintf(`#!/bin/sh
n=$(grep -c function_call_output "$A10N_MOCK_SESSION_FILE")
call=$(sed -n "$((n+1))p" <<'CALLS_EOF'
%s
CALLS_EOF
)
if [ -n "$call" ]; then
  printf '%%s\n' "$call" | sed "s/IDPLACE/call_%s_$n/"
  exit 0
fi
text='%s'
printf '%%s\n' "$(jq -nc --argjson t "$text" '{type:"assistant",message:{content:[{type:"text",text:$t}]}}')" "$(jq -nc --argjson t "$text" '{type:"result",subtype:"success",result:$t}')"
`, strings.Join(lines, "\n"), tag, strings.ReplaceAll(string(fin), "'", `'\''`))
}
