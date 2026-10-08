package replay

import (
	"fmt"
	"strings"
)

// promptAt is where, from `from` on, the transcript holds this prompt: the user message of it. A
// slash command is recorded as a <command-name> message after what the command did, so a compaction
// (/compact) is found from its compact_boundary, one record ahead of which the run's records begin.
func promptAt(recs []map[string]any, from int, prompt string) int {
	for j := from; j < len(recs); j++ {
		if !isPrompt(recs[j], prompt) {
			continue
		}
		if strings.HasPrefix(prompt, "/") {
			for k := j; k >= from; k-- {
				if recs[k]["type"] == "system" && recs[k]["subtype"] == "compact_boundary" {
					return k - 1
				}
			}
		}
		return j
	}
	return -1
}

// isPrompt is whether a transcript record is the user's message of this prompt.
func isPrompt(rec map[string]any, prompt string) bool {
	if rec["type"] != "user" {
		return false
	}
	msg, _ := rec["message"].(map[string]any)
	if s, ok := msg["content"].(string); ok {
		if strings.HasPrefix(prompt, "/") {
			return strings.Contains(s, "<command-name>"+strings.Fields(prompt)[0]+"</command-name>")
		}
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
		at[i] = promptAt(recs, from, sp.prompt)
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
