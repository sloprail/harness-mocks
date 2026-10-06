package replay

import "strings"

// compactionMarker is the role of the record the adapter puts in a transcript where
// the harness compacted: the compaction's own fields (the preCompact hook's).
const compactionMarker = "compact"

// compactionCommon are the keys of a preCompact payload every payload has or that
// differ in every run; the rest say how big the context was when it was compacted.
var compactionCommon = map[string]bool{
	"conversation_id": true, "generation_id": true, "session_id": true, "hook_event_name": true,
	"cursor_version": true, "workspace_roots": true, "user_email": true, "transcript_path": true,
}

// compaction is one preCompact of the recording's hook log: what its payload says of
// the context, and how many of the conversation's tool calls had finished when it fired.
type compaction struct {
	fields map[string]any
	after  int
}

// compactionsOf are the preCompacts the recording's hook log holds, in order. The
// transcript does not say where the model was when the harness compacted (it rewrites
// the prompt at a place of its own, recorded: runs/compaction-transcript-continuity,
// where it is three responses before the hook), so the hook log's order places it.
func compactionsOf(payloads []map[string]any) []compaction {
	var out []compaction
	done := map[any]int{}
	for _, p := range payloads {
		switch p["hook_event_name"] {
		case "postToolUse", "postToolUseFailure":
			done[p["conversation_id"]]++
		case "preCompact":
			fields := map[string]any{}
			for k, v := range p {
				if !compactionCommon[k] {
					fields[k] = v
				}
			}
			out = append(out, compaction{fields, done[p["conversation_id"]]})
		}
	}
	return out
}

// markCompactions puts a marker where the hook log shows the harness compacted. The
// transcript shows it too, by writing the prompt again, byte for byte, after the
// records it dropped, and (once the context has been searched for tools) a message
// that lists the tools it can discover before it. Those records are not turns of the
// model or of the user, so they are taken out, and the k-th preCompact is matched to
// the k-th re-written prompt. A transcript whose compactions and re-written prompts
// are not as many is not read.
func markCompactions(records []map[string]any, marks []compaction) ([]map[string]any, error) {
	if len(marks) == 0 {
		return records, nil
	}
	var kept []map[string]any
	first, rewritten := "", 0
	for _, rec := range records {
		if rec["role"] != "user" {
			kept = append(kept, rec)
			continue
		}
		q := firstQuery([]map[string]any{rec})
		switch {
		case first == "" && q != "":
			first = q
			kept = append(kept, rec)
		case q == "" && isDynamicTools(rec): // the message ahead of a re-written prompt
		case q == first:
			rewritten++
		default:
			kept = append(kept, rec)
		}
	}
	if rewritten != len(marks) {
		return nil, unbuildable("the harness compacted %d times and the transcript shows its prompt written again %d times", len(marks), rewritten)
	}
	var out []map[string]any
	calls, k := 0, 0
	for _, rec := range kept {
		for k < len(marks) && rec["role"] == "assistant" && calls >= marks[k].after {
			out = append(out, map[string]any{"role": compactionMarker, "fields": marks[k].fields})
			k++
		}
		out = append(out, rec)
		calls += toolUses(rec)
	}
	if k != len(marks) {
		return nil, unbuildable("the harness compacted after %d tool calls and the transcript has fewer", marks[k].after)
	}
	return out, nil
}

// toolUses is how many tool calls an assistant record makes.
func toolUses(rec map[string]any) int {
	n := 0
	msg, _ := rec["message"].(map[string]any)
	blocks, _ := msg["content"].([]any)
	for _, b := range blocks {
		if block, _ := b.(map[string]any); block["type"] == "tool_use" {
			n++
		}
	}
	return n
}

// isDynamicTools reports whether a user record is the message that lists the tools
// the agent can discover.
func isDynamicTools(rec map[string]any) bool {
	msg, _ := rec["message"].(map[string]any)
	blocks, _ := msg["content"].([]any)
	for _, b := range blocks {
		block, _ := b.(map[string]any)
		if text, _ := block["text"].(string); strings.HasPrefix(text, "<dynamic_tools>") {
			return true
		}
	}
	return false
}
