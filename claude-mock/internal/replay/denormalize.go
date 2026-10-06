package replay

import (
	"encoding/json"
	"fmt"
	"strings"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// Denormalize turns the unified recording into the mock's scenario: the
// unified tools go back to Claude's names, and each sub-agent's turns become a
// script of their own, which the spawning call names in its `script` input.
// dir is where the scripts will be written (an absolute path).
func Denormalize(rec core.Recording, dir string) Scenario {
	scripts := map[string]string{}
	n := 0
	var scriptFor func(tag string, a core.Agent, parent *core.Agent, above []*core.Agent, skip int) string
	scriptFor = func(tag string, a core.Agent, parent *core.Agent, above []*core.Agent, skip int) string {
		calls := make([]scriptCall, len(a.Calls))
		gates := core.Gates(a, parent, above...)
		for i, c := range a.Calls {
			calls[i] = mockCall(c)
			calls[i].Gate = gates[i]
			if c.Tool == core.ToolSpawn && c.Sub != nil {
				name := fmt.Sprintf("sub%d.sh", n)
				subTag := fmt.Sprintf("sub%d", n)
				n++
				scripts[name] = scriptFor(subTag, *c.Sub, &a, append(aboveOf(parent), above...), 0)
				calls[i].Input["script"] = dir + "/" + name
			}
		}
		extra := ""
		if tag == "main" {
			extra = rec.Setup["result"]
		}
		return script(tag, calls, a.Final, extra, skip, gates[len(calls)])
	}
	var earlier []ScenarioEarlier
	before := 0
	for i, e := range earlierOf(rec) {
		name := fmt.Sprintf("pre%d.sh", i)
		scripts[name] = scriptFor(fmt.Sprintf("pre%d", i), e.Agent, nil, nil, 0)
		earlier = append(earlier, ScenarioEarlier{Prompt: e.Prompt, Script: name})
		before += len(e.Agent.Calls)
	}
	// a run that resumes a session finds the earlier runs' tool results in its file: the script
	// skips as many calls as they made. (A run of a session of its own skips none.)
	skipMain := 0
	if resumes(strings.Fields(rec.Setup["args"])) {
		skipMain = before
	}
	main := scriptFor("main", rec.Agent, nil, nil, skipMain)
	var then []ScenarioStep
	done := before + len(rec.Agent.Calls)
	var files []stepFiles
	if text := rec.Setup["steps"]; text != "" {
		_ = json.Unmarshal([]byte(text), &files)
	}
	for i, st := range rec.Then {
		skip := 0
		if !startsOwnSession(st.Args) {
			skip = done
		}
		done += len(st.Agent.Calls)
		step := ScenarioStep{Script: scriptFor(fmt.Sprintf("step%d", i+1), st.Agent, nil, nil, skip), Prompt: st.Prompt, Args: st.Args, Cwd: st.Cwd}
		if i < len(files) {
			step.Symlink, step.Settings, step.Hook = files[i].Symlink, files[i].Settings, files[i].Hook
		}
		then = append(then, step)
	}
	return Scenario{
		Settings: rec.Setup["settings.json"],
		Hook:     rec.Setup["hook.sh"],
		Scripts:  scripts,
		Earlier:  earlier,
		Script:   main,
		Prompt:   rec.Prompt,
		Then:     then,
	}
}

// resumes is whether a run's flags resume, continue or fork an earlier session.
func resumes(args []string) bool { return hasFlag(args, "--resume") || hasFlag(args, "--continue") }

// startsOwnSession is whether a later run's flags start a session of its own (--session-id, with
// nothing resumed or forked), not the earlier one.
func startsOwnSession(args []string) bool {
	return hasFlag(args, "--session-id") && !resumes(args)
}

func hasFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

// aboveOf is the agent that started the agent being scripted, as one of the agents above its sub-agents.
func aboveOf(parent *core.Agent) []*core.Agent {
	if parent == nil {
		return nil
	}
	return []*core.Agent{parent}
}
