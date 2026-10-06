package replay

import (
	"encoding/json"
	"strings"

	"github.com/sloprail/harness-mocks/internal/scenario"
)

// finalLines is what the script prints to end: the answer and the result, which
// is an error when the recorded one was (extra: its is_error and api_error_status,
// what the mock cannot know of a model API that failed).
func finalLines(final, extra string) string {
	var parts []string
	var more map[string]any
	_ = json.Unmarshal([]byte(extra), &more)
	if final != "" {
		parts = append(parts, assistantFrame(map[string]any{"type": "text", "text": final}, more["assistant"]))
	}
	frame := map[string]any{"type": "result", "subtype": "success", "result": final}
	for k, v := range more {
		if k != "assistant" {
			frame[k] = v
		}
	}
	result, _ := json.Marshal(frame)
	return strings.Join(append(parts, string(result)), "\x01")
}

func assistantFrame(block map[string]any, extra ...any) string {
	msg := map[string]any{"role": "assistant", "content": []any{block}}
	frame := map[string]any{"type": "assistant", "message": msg}
	if len(extra) > 0 { // what the harness added to a message it wrote itself (an API error): its flags and the message's stop reason
		if e, ok := extra[0].(map[string]any); ok {
			for k, v := range e {
				if k == "stop_reason" || k == "stop_sequence" {
					msg[k] = v
				} else {
					frame[k] = v
				}
			}
		}
	}
	b, _ := json.Marshal(frame)
	return string(b)
}

// callLine is what the script prints for one call: what the model said before
// it (a frame of its own, as the real stream has it) and the call, joined by
// the byte \001, which JSON never holds raw; the script splits them again.
func callLine(id string, c scriptCall) string {
	if c.Silent { // a response with no visible output: thinking and the result that ends the turn
		return gateLine(c.Gate) + assistantFrame(map[string]any{"type": "thinking", "thinking": ""}) + "\x01" + finalLines("", "")
	}
	if c.Name == "" { // an answer the end of the turn was refused on: the text and the result that ends the turn
		return gateLine(c.Gate) + finalLines(c.Reply, "")
	}
	var parts []string
	if c.Text != nil {
		parts = append(parts, assistantFrame(map[string]any{"type": "text", "text": *c.Text}))
	}
	block := map[string]any{"type": "tool_use", "id": id, "name": c.Name, "input": c.Input}
	if c.More {
		block["more"] = true // another call of the same message follows (the scenario format's marker)
	}
	parts = append(parts, assistantFrame(block))
	return gateLine(c.Gate) + strings.Join(parts, "\x01")
}

// gateLine is the control line that holds the step back until what its gate names has happened
// (scenario.Gate), with the byte that joins the step's lines after it; empty when the step waits for nothing.
func gateLine(g scenario.Gate) string {
	if g.None() {
		return ""
	}
	b, _ := json.Marshal(map[string]any{"type": "gate", "gate": g})
	return string(b) + "\x01"
}
