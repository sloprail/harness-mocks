package replay

import (
	"fmt"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// Scenario is what the mock is given to replay a recording: the run's own
// setup, and the scenario scripts that play the model, in the format the mock
// takes of any scenario.
type Scenario struct {
	// Scripts are the sub-agents' scripts, by file name; the main one is Script.
	Scripts map[string]string
	Script  string
	Prompt  string
}

// step is one response of the model in the mock's script vocabulary: what it
// said, and the calls it made together (none: an answer).
type step struct {
	said    *string
	calls   []scriptCall
	thought *core.Thinking // what the model thought in the response, when recorded
}

// lines is how many assistant records the mock writes to the session file for
// the step: one for the text, one for each call.
func (s step) lines() int {
	n := len(s.calls)
	if s.said != nil {
		n++
	}
	return n
}

// scriptCall is one tool call in the mock's script vocabulary.
type scriptCall struct {
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

// Denormalize turns the unified recording into the mock's scenario: the unified
// tools go back to the names the mock's script takes, and each sub-agent's turns
// become a script of their own, which the spawning call names in its `script`
// input. dir is where the scripts will be written (an absolute path). A capture
// writes the run's own paths into what it records as <RUN>, <TMP> and
// <RUN_DIRNAME> (rules.go); paths says what they stand for in this replay, and a
// nil paths leaves them as recorded.
func Denormalize(rec core.Recording, dir string, paths *Paths) Scenario {
	scripts := map[string]string{}
	n := 0
	var scriptFor func(tag string, a core.Agent) string
	scriptFor = func(tag string, a core.Agent) string {
		var steps []step
		for _, c := range a.Calls {
			switch {
			case c.Tool == core.ToolAnswer:
				text, _ := c.Input["text"].(string)
				steps = append(steps, step{said: &text, thought: c.Thinking})
			case c.SameTurn && len(steps) > 0:
				steps[len(steps)-1].calls = append(steps[len(steps)-1].calls, mockCall(c))
			default:
				steps = append(steps, step{said: c.Said, calls: []scriptCall{mockCall(c)}, thought: c.Thinking})
			}
			if c.Tool == core.ToolSpawn && c.Sub != nil {
				name := fmt.Sprintf("sub%d.sh", n)
				subTag := fmt.Sprintf("sub%d", n)
				n++
				scripts[name] = scriptFor(subTag, *c.Sub)
				last := steps[len(steps)-1]
				last.calls[len(last.calls)-1].Input["script"] = dir + "/" + name
			}
		}
		if a.Final != "" {
			steps = append(steps, step{said: &a.Final, thought: a.FinalThinking})
		}
		return script(tag, paths.expandSteps(steps))
	}
	return Scenario{Scripts: scripts, Script: scriptFor("main", rec.Agent), Prompt: rec.Prompt}
}
