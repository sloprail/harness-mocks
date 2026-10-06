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
