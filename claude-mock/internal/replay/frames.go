package replay

// unmodelled are the frames of the real stream that say nothing the mock could
// be told to say: the real run's own tools, commands and model, the account's
// rate limits, the model's thinking estimates. They are left out of both sides.
// What a recording shows of them is not replayed: the mock writes an init frame
// only after a compaction, never at a session's start, and that one is dropped
// too, so the cells' statements about init have no replay evidence yet.
var unmodelled = map[string]bool{
	"system/init":             true, // the real run's tools, skills, slash commands and model
	"system/commands_changed": true, // the account's slash commands
	"system/thinking_tokens":  true, // the model's estimate of its own thinking
	"rate_limit_event/":       true, // the account's rate limits
}

// Frames are the stream's frames as they are compared: the unmodelled ones
// dropped, and an assistant frame without its thinking blocks (the mock has no
// model and never thinks; a frame of nothing but thinking is not compared) and
// without the API response's own bookkeeping (assistantMeta, assistantEmpty).
func Frames(frames []map[string]any) []map[string]any {
	var out []map[string]any
	for _, f := range frames {
		typ, _ := f["type"].(string)
		sub, _ := f["subtype"].(string)
		if unmodelled[typ+"/"+sub] {
			continue
		}
		if typ == "assistant" {
			var ok bool
			if f, ok = assistant(f); !ok {
				continue
			}
		}
		out = append(out, f)
	}
	return out
}

// assistantMeta are the keys of an assistant frame's message that belong to the
// model API's response, not to what the model said or called: its id, model and
// type, and the cache diagnostics.
var assistantMeta = map[string]bool{"id": true, "model": true, "type": true, "diagnostics": true}

// assistantEmpty are the keys of the message that are left out only when empty
// (null or an empty list), as the mock has none: a recording that gives one a
// value shows it as a difference.
var assistantEmpty = map[string]bool{
	"container": true, "stop_reason": true, "stop_sequence": true, "stop_details": true,
	"context_management": true, "input_transformations": true,
}

func empty(v any) bool {
	if l, ok := v.([]any); ok {
		return len(l) == 0
	}
	return v == nil
}

// assistant is the assistant frame without its thinking blocks and the
// response's bookkeeping, a copy; false when no block is left.
func assistant(f map[string]any) (map[string]any, bool) {
	msg, _ := f["message"].(map[string]any)
	var content []any
	blocks, _ := msg["content"].([]any)
	for _, b := range blocks {
		block, _ := b.(map[string]any)
		if block["type"] == "thinking" {
			continue
		}
		kept := map[string]any{}
		for k, v := range block {
			if k != "caller" { // how the tool was called (directly, here): not what was called
				kept[k] = v
			}
		}
		if input, ok := kept["input"].(map[string]any); ok && hasMockKeys(kept["name"]) {
			kept["input"] = withoutScript(input)
		}
		content = append(content, kept)
	}
	if len(content) == 0 {
		return nil, false
	}
	message := map[string]any{}
	for k, v := range msg {
		if !assistantMeta[k] && !(assistantEmpty[k] && empty(v)) {
			message[k] = v
		}
	}
	message["content"] = content
	out := map[string]any{}
	for k, v := range f {
		out[k] = v
	}
	out["message"] = message
	if wire, ok := f["wire_tool_inputs"].(map[string]any); ok { // by call id: an Agent call's input has the mock's script key too
		w := map[string]any{}
		for id, in := range wire {
			if m, ok := in.(map[string]any); ok {
				in = withoutScript(m)
			}
			w[id] = in
		}
		out["wire_tool_inputs"] = w
	}
	return out, true
}

// hasMockKeys is a tool whose input may carry a key of the mock's own: the sub-agent's script of an
// Agent call, whether a Bash call leaves task frames, the result a script gives a web call.
func hasMockKeys(name any) bool {
	return name == "Agent" || name == "Task" || name == "Bash" || name == "WebFetch" || name == "WebSearch"
}

// withoutScript is a call's input without the mock's own keys, `script` (the sub-agent's script) and
// `task_frames` (whether a Bash call leaves task frames), which the real tools have no key for; a copy.
func withoutScript(input map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range input {
		if k != "script" && k != "mock_start_after_post" && k != "task_frames" && k != "mock_result" {
			out[k] = v
		}
	}
	return out
}

// HookPayloads are the hook payloads as they are compared: an Agent call's
// tool_input without the mock's own script key.
func HookPayloads(payloads []map[string]any) []map[string]any {
	out := make([]map[string]any, len(payloads))
	for i, p := range payloads {
		out[i] = p
		if input, ok := p["tool_input"].(map[string]any); ok && hasMockKeys(p["tool_name"]) {
			c := map[string]any{}
			for k, v := range p {
				c[k] = v
			}
			c["tool_input"] = withoutScript(input)
			out[i] = c
		}
	}
	return out
}
