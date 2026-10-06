package replay

import "strings"

// Paths are the replay's own: where the capture wrote <RUN>, <TMP> and
// <RUN_DIRNAME>.
type Paths struct{ Run, Tmp, RunDirname string }

// expand is v with the placeholders a capture sanitised paths into replaced, in
// every string it holds; the order is the capture's own (the run first, as it
// holds the temp root).
func (p *Paths) expand(v any) any {
	switch x := v.(type) {
	case string:
		if p == nil {
			return x
		}
		return strings.NewReplacer("<RUN_DIRNAME>", p.RunDirname, "<RUN>", p.Run, "<TMP>", p.Tmp).Replace(x)
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = p.expand(e)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = p.expand(e)
		}
		return out
	}
	return v
}

func (p *Paths) expandSteps(steps []step) []step {
	out := make([]step, len(steps))
	for i, s := range steps {
		out[i] = step{calls: make([]scriptCall, len(s.calls)), thought: s.thought, compact: s.compact}
		if s.said != nil {
			text := p.expand(*s.said).(string)
			out[i].said = &text
		}
		for j, c := range s.calls {
			out[i].calls[j] = scriptCall{Name: c.Name, Input: p.expand(c.Input).(map[string]any)}
		}
	}
	return out
}
