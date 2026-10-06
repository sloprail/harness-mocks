package replay

import (
	"strings"
)

func blockID(block map[string]any) string {
	id, _ := block["id"].(string)
	return id
}

// endsTurn is whether a record opens a turn of the model's own accord: a user record whose content is
// text (a task's notification, a hook's feedback), not a tool's result.
func endsTurn(rec map[string]any) bool {
	if rec["type"] != "user" {
		return false
	}
	msg, _ := rec["message"].(map[string]any)
	_, text := msg["content"].(string)
	return text
}

// nudge starts the user record the harness leaves when a model's response had no visible output.
const nudge = "[Your previous response had no visible output."

// isNudge is whether a user record is that nudge.
func isNudge(rec map[string]any) bool {
	msg, _ := rec["message"].(map[string]any)
	s, _ := msg["content"].(string)
	return strings.HasPrefix(s, nudge)
}

// withKey is the input with one more key, a copy.
func withKey(in map[string]any, k string, v any) map[string]any {
	out := make(map[string]any, len(in)+1)
	for key, val := range in {
		out[key] = val
	}
	out[k] = v
	return out
}
