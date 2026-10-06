package replay

import (
	"fmt"
	"strings"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// Scenario is what the mock is given to replay a recording: the run's own
// setup (its settings, its hook command, its prompt) and the scenario script
// that makes the model's calls, in the format the mock takes of any scenario.
type Scenario struct {
	Settings string
	Hook     string
	// Scripts are the sub-agents' scripts, by file name; the main one is Script.
	Scripts map[string]string
	Script  string
	Prompt  string
	// Then are the later runs of claude, each with its own script and the flags to run the mock with.
	Then []ScenarioStep
}

// ScenarioStep is a later run: its script, its prompt and the words given before it (a resume's
// <SESSION> is the first run's session).
type ScenarioStep struct {
	Script, Prompt string
	Args           []string
}

// scriptCall is one tool call the model made, in the mock's script vocabulary.
type scriptCall struct {
	Text  *string        `json:"text,omitempty"` // what the model said just before the call, if it said anything
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

// Denormalize turns the unified recording into the mock's scenario: the
// unified tools go back to Claude's names, and each sub-agent's turns become a
// script of their own, which the spawning call names in its `script` input.
// dir is where the scripts will be written (an absolute path).
func Denormalize(rec core.Recording, dir string) Scenario {
	scripts := map[string]string{}
	n := 0
	var scriptFor func(tag string, a core.Agent, skip int) string
	scriptFor = func(tag string, a core.Agent, skip int) string {
		calls := make([]scriptCall, len(a.Calls))
		for i, c := range a.Calls {
			calls[i] = mockCall(c)
			if c.Tool == core.ToolSpawn && c.Sub != nil {
				name := fmt.Sprintf("sub%d.sh", n)
				subTag := fmt.Sprintf("sub%d", n)
				n++
				scripts[name] = scriptFor(subTag, *c.Sub, 0)
				calls[i].Input["script"] = dir + "/" + name
			}
		}
		extra := ""
		if tag == "main" {
			extra = rec.Setup["result"]
		}
		return script(tag, calls, a.Final, extra, skip)
	}
	main := scriptFor("main", rec.Agent, 0)
	var then []ScenarioStep
	// a run that resumes a session finds the earlier runs' tool results in its file: the script
	// skips as many calls as they made. (A run of a session of its own skips none.)
	done := len(rec.Agent.Calls)
	for i, st := range rec.Then {
		skip := 0
		if !startsOwnSession(st.Args) {
			skip = done
		}
		done += len(st.Agent.Calls)
		then = append(then, ScenarioStep{Script: scriptFor(fmt.Sprintf("step%d", i+1), st.Agent, skip), Prompt: st.Prompt, Args: st.Args})
	}
	return Scenario{
		Settings: rec.Setup["settings.json"],
		Hook:     rec.Setup["hook.sh"],
		Scripts:  scripts,
		Script:   main,
		Prompt:   rec.Prompt,
		Then:     then,
	}
}

// mockCall is Claude's name for a unified call; the input is copied, as the
// spawn's script parameter is added to it. The spawn's message is the prompt
// the input already carries.
func mockCall(c core.Call) scriptCall {
	name := "Bash"
	switch c.Tool {
	case core.ToolSpawn:
		name = "Agent"
	case toolRead:
		name = "Read"
	}
	in := make(map[string]any, len(c.Input))
	for k, v := range c.Input {
		if k != "message" {
			in[k] = v
		}
	}
	return scriptCall{Text: c.Said, Name: name, Input: in}
}

// script is the mock script that makes the given calls, one per turn, then
// answers with final. The calls are inside it, so a sub-agent's script is a
// file of its own. The mock runs the script once per tool call and the session
// file holds the results so far, so the script's n-th run makes the n-th call.
// Call ids are unique across the run's scripts (tag), as the real ones are.
func script(tag string, calls []scriptCall, final, extra string, skip int) string {
	lines := make([]string, 0, len(calls)+1)
	for i, c := range calls {
		lines = append(lines, callLine(fmt.Sprintf("toolu_%s%d", tag, i), c))
	}
	lines = append(lines, finalLines(final, extra))
	return fmt.Sprintf(`#!/bin/sh
n=$(grep -c '"type":"tool_result"' "$A10N_MOCK_SESSION_FILE")
sed -n "$((n+1-%d))p" <<'CALLS_EOF' | tr '\001' '\n'
%s
CALLS_EOF
`, skip, strings.Join(lines, "\n"))
}

// startsOwnSession is whether a later run's flags start a session of its own (--session-id), not
// the earlier one.
func startsOwnSession(args []string) bool {
	for _, a := range args {
		if a == "--session-id" {
			return true
		}
	}
	return false
}
