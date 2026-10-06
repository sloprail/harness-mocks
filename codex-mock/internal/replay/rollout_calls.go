package replay

import "math"

// RolloutCall is a tool call the model made in a recorded rollout, in the recording's own names: the
// tool (exec_command, multi_agent_v1__spawn_agent, apply_patch ...) and its arguments as JSON values.
// An argument that is not an object (apply_patch's patch) is Input["arg"].
type RolloutCall struct {
	Tool  string
	Input map[string]any
}

// RolloutCalls are the tool calls of the model's scripts of one rollout, in order. A script the
// adapter cannot read (its calls depend on a result) contributes none.
func RolloutCalls(records []map[string]any) (out []RolloutCall) {
	js := newJSRun()
	for _, rec := range records {
		p, _ := rec["payload"].(map[string]any)
		if rec["type"] != "response_item" || p["type"] != "custom_tool_call" {
			continue
		}
		src, _ := p["input"].(string)
		made, err := js.script(src)
		if err != nil {
			continue
		}
		for _, m := range made {
			in := map[string]any{}
			if len(m.Args) == 1 {
				if obj, ok := m.Args[0].(map[string]any); ok {
					for k, v := range obj {
						in[k] = plain(v)
					}
				} else {
					in["arg"] = plain(m.Args[0])
				}
			}
			out = append(out, RolloutCall{Tool: m.Name, Input: in})
		}
	}
	return out
}

// plain is a value of an evaluated script as the JSON a call is made of.
func plain(v any) any {
	switch x := v.(type) {
	case number:
		if x.f == math.Trunc(x.f) && math.Abs(x.f) <= 1<<31 {
			return float64(int(x.f))
		}
		return x.f
	case map[string]any:
		m := map[string]any{}
		for k, e := range x {
			m[k] = plain(e)
		}
		return m
	case []any:
		l := make([]any, len(x))
		for i, e := range x {
			l[i] = plain(e)
		}
		return l
	}
	return v
}
