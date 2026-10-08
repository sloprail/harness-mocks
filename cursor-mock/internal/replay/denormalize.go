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
	// Lines is how many assistant records the main script adds to the session's
	// file: a later step that resumes the session starts after them.
	Lines int
}

// step is one response of the model in the mock's script vocabulary: what it
// said, and the calls it made together (none: an answer).
type step struct {
	said    *string
	calls   []scriptCall
	thought *core.Thinking // what the model thought in the response, when recorded
	compact map[string]any // the compaction the harness made just before the response, when it did
}

// lines is how many assistant records the mock writes to the session file for
// the step: one for each call, the text before the first being in its record
// (recorded: a step's text and its call are one record), and one for the text of
// a step that makes no call.
func (s step) lines() int {
	if len(s.calls) > 0 {
		return len(s.calls)
	}
	if s.said != nil {
		return 1
	}
	return 0
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
	return denormalize(rec.Agent, rec.Prompt, dir, paths, "", 0)
}

// DenormalizeStep is the scenario of the i-th later step of a run (1 is the first
// after the opening one): its scripts and the ids of its calls are named apart from
// the other steps'.
// after is how many assistant records the session's file already holds when it
// begins (the earlier steps', when it resumes their session).
func DenormalizeStep(st core.Step, i int, dir string, paths *Paths, after int) Scenario {
	return denormalize(st.Agent, st.Prompt, dir, paths, fmt.Sprintf("s%d_", i), after)
}

func denormalize(root core.Agent, prompt, dir string, paths *Paths, prefix string, after int) Scenario {
	scripts := map[string]string{}
	n := 0
	lines := 0
	var scriptFor func(tag string, a core.Agent, after int) string
	scriptFor = func(tag string, a core.Agent, after int) string {
		var steps []step
		for _, c := range a.Calls {
			switch {
			case c.Tool == core.ToolAnswer:
				text, _ := c.Input["text"].(string)
				steps = append(steps, step{said: &text, thought: c.Thinking, compact: c.Compact})
			case c.SameTurn && len(steps) > 0:
				steps[len(steps)-1].calls = append(steps[len(steps)-1].calls, mockCall(c))
			default:
				steps = append(steps, step{said: c.Said, calls: []scriptCall{mockCall(c)}, thought: c.Thinking, compact: c.Compact})
			}
			if c.Tool == core.ToolSpawn && c.Sub != nil {
				name := fmt.Sprintf("%ssub%d.sh", prefix, n)
				subTag := fmt.Sprintf("%ssub%d", prefix, n)
				n++
				scripts[name] = scriptFor(subTag, *c.Sub, 0)
				last := steps[len(steps)-1]
				last.calls[len(last.calls)-1].Input["script"] = dir + "/" + name
				if c.Sub.ID != "" { // a reply may quote the sub-agent's id: it takes the recorded one
					last.calls[len(last.calls)-1].Input["agent_id"] = c.Sub.ID
				}
			}
		}
		if a.Final != "" {
			steps = append(steps, step{said: &a.Final, thought: a.FinalThinking})
		}
		if tag == prefix+"main" {
			for _, st := range steps {
				lines += st.lines()
			}
		}
		return script(tag, paths.expandSteps(steps), after)
	}
	main := scriptFor(prefix+"main", root, after)
	return Scenario{Scripts: scripts, Script: main, Prompt: prompt, Lines: lines}
}
