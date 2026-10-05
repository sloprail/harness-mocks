package replay

// unmodelled are the frames of the real stream the mock has no model to say:
// the model's thinking, which the mock never does (the noninteractive-run cell
// declares it).
var unmodelled = map[string]bool{"thinking": true}

// Frames are the stream's frames as they are compared: the thinking dropped.
func Frames(frames []map[string]any) []map[string]any {
	var out []map[string]any
	for _, f := range frames {
		if typ, _ := f["type"].(string); !unmodelled[typ] {
			out = append(out, f)
		}
	}
	return out
}

// modelledHooks drops from the recording's hook log what the hooks logged for
// the events the mock never fires (adr/modeled-surface): the model's thinking, as
// with the stream. That is the payloads of the event and the lines a hook script
// wrote for it.
func modelledHooks(payloads []map[string]any) []map[string]any {
	var out []map[string]any
	for _, p := range payloads {
		if eventOf(p) != "afterAgentThought" {
			out = append(out, p)
		}
	}
	return out
}
