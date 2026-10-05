package replay

import (
	"encoding/json"
	"fmt"
	"strings"

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
	said  *string
	calls []scriptCall
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
				steps = append(steps, step{said: &text})
			case c.SameTurn && len(steps) > 0:
				steps[len(steps)-1].calls = append(steps[len(steps)-1].calls, mockCall(c))
			default:
				steps = append(steps, step{said: c.Said, calls: []scriptCall{mockCall(c)}})
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
			steps = append(steps, step{said: &a.Final})
		}
		return script(tag, paths.expandSteps(steps))
	}
	return Scenario{Scripts: scripts, Script: scriptFor("main", rec.Agent), Prompt: rec.Prompt}
}

// mockCall is the mock's script name and input for a unified call; the input is
// copied, as the spawn's script parameter is added to it.
func mockCall(c core.Call) scriptCall {
	in := make(map[string]any, len(c.Input))
	for k, v := range c.Input {
		in[k] = v
	}
	name := "Shell"
	switch c.Tool {
	case core.ToolSpawn:
		name = "Task"
		delete(in, "message")
	case core.ToolReadFile:
		name = "Read"
		in["file_path"] = in["path"]
		delete(in, "path")
	case core.ToolWriteFile:
		name = "Write"
		in["file_path"] = in["path"]
		delete(in, "path")
	case core.ToolSearchFiles:
		name = "Grep"
	case core.ToolDeleteFile:
		name = "Delete"
		in["file_path"] = in["path"]
		delete(in, "path")
	case core.ToolMCP:
		name = fmt.Sprintf("mcp__%v__%v", in["server"], in["tool"])
		description := in["description"]
		in, _ = in["arguments"].(map[string]any)
		if in == nil {
			in = map[string]any{}
		}
		if description != nil { // the mock takes the model's description of the call in a key of its own
			in["__description"] = description
		}
	}
	return scriptCall{Name: name, Input: in}
}

// script is the mock script that plays the steps. The mock runs it once per
// turn and the session file holds the agent's records so far, so the step to
// play is the one that starts where the file stands: each step adds as many
// assistant records as its lines say. Past the last step it prints nothing,
// which ends the run. Call ids are unique across the run's scripts (tag), as
// the real ones are.
func script(tag string, steps []step) string {
	var b strings.Builder
	b.WriteString("#!/bin/sh\nn=$(grep -c '\"role\":\"assistant\"' \"$A10N_MOCK_SESSION_FILE\" 2>/dev/null)\ncase ${n:-0} in\n")
	at := 0
	for i, s := range steps {
		fmt.Fprintf(&b, "%d) printf '%%s\\n' '%s' ;;\n", at, shellQuote(stepLine(fmt.Sprintf("toolu_%s_%d", tag, i), s)))
		at += s.lines()
	}
	b.WriteString("esac\n")
	return b.String()
}

// stepLine is the assistant line the script prints for a step: the text the
// model said, then its calls, as the blocks of one record.
func stepLine(id string, s step) string {
	var blocks []any
	if s.said != nil {
		blocks = append(blocks, map[string]any{"type": "text", "text": *s.said})
	}
	for j, c := range s.calls {
		blocks = append(blocks, map[string]any{"type": "tool_use", "id": fmt.Sprintf("%s_%d", id, j), "name": c.Name, "input": c.Input})
	}
	b, _ := json.Marshal(map[string]any{"type": "assistant", "message": map[string]any{"role": "assistant", "content": blocks}})
	return string(b)
}

// shellQuote is s as the inside of a single-quoted shell word.
func shellQuote(s string) string { return strings.ReplaceAll(s, "'", `'\''`) }
