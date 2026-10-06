package replay

import (
	"encoding/json"
	"strings"
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
	if c.Control { // a control record the mock reads: {"type":"compact","summary":…,"trigger":…}
		rec := map[string]any{"type": c.Name}
		for k, v := range c.Input {
			rec[k] = v
		}
		b, _ := json.Marshal(rec)
		return string(b)
	}
	if c.Answer { // what it said before, then the answer and the result that ends the turn
		var early []string
		for _, e := range c.Early {
			early = append(early, assistantFrame(map[string]any{"type": "text", "text": e}))
		}
		return strings.Join(append(early, finalLines(*c.Text, "")), "\x01")
	}
	var parts []string
	for _, e := range c.Early {
		parts = append(parts, assistantFrame(map[string]any{"type": "text", "text": e}))
	}
	if c.Text != nil {
		parts = append(parts, assistantFrame(map[string]any{"type": "text", "text": *c.Text}))
	}
	parts = append(parts, assistantFrame(map[string]any{"type": "tool_use", "id": id, "name": c.Name, "input": c.Input}))
	return strings.Join(parts, "\x01")
}
