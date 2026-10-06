package replay

import (
	"fmt"
	"strings"
)

// isPrompt is whether a transcript record is the user's message of this prompt.
func isPrompt(rec map[string]any, prompt string) bool {
	if rec["type"] != "user" {
		return false
	}
	msg, _ := rec["message"].(map[string]any)
	if s, ok := msg["content"].(string); ok {
		return strings.TrimSpace(s) == prompt
	}
	blocks, _ := msg["content"].([]any)
	for _, b := range blocks {
		if m, _ := b.(map[string]any); m != nil && m["type"] == "text" && strings.TrimSpace(fmt.Sprint(m["text"])) == prompt {
			return true
		}
	}
	return false
}

// stepRecords are the records each run made: a session worked in more than once holds the runs one
// after the other, each after the user message of its own prompt.
func stepRecords(specs []stepSpec, threads []string, sessions map[string][]map[string]any) ([][]map[string]any, error) {
	at := make([]int, len(specs))
	last := map[string]int{}
	for i, sp := range specs {
		recs, from := sessions[threads[i]], 0
		if l, seen := last[threads[i]]; seen {
			from = l + 1
		}
		at[i] = -1
		for j := from; j < len(recs) && at[i] < 0; j++ {
			if isPrompt(recs[j], sp.prompt) {
				at[i] = j
			}
		}
		if at[i] < 0 {
			return nil, unbuildable(fmt.Errorf("run %d: its prompt is not in the transcript of session %s", i, threads[i]))
		}
		last[threads[i]] = at[i]
	}
	out := make([][]map[string]any, len(specs))
	for i := range specs {
		recs, end := sessions[threads[i]], len(sessions[threads[i]])
		for j := i + 1; j < len(specs); j++ {
			if threads[j] == threads[i] {
				end = at[j]
				break
			}
		}
		out[i] = recs[at[i]+1 : end]
	}
	return out, nil
}
