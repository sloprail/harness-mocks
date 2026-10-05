package replay

import (
	"regexp"
	"strings"

	rp "github.com/sloprail/harness-mocks/internal/replay"
)

// Rules are what a recording and a replay of it may differ in, and why: no
// capability cell is about any of it. repo and work are the replay's own paths,
// which the recording has as the capture wrote them: <RUN>, <TMP>, and the run
// directory as claude encodes it into a folder name, <RUN_DIRNAME>. taskIDs
// are the run's task and agent ids (see RunIDs), which have no pattern.
func Rules(repo, work string, taskIDs []string) rp.Rules {
	re := regexp.MustCompile
	enc := re(`[^A-Za-z0-9]`).ReplaceAllString(repo, "-")
	ids := []*regexp.Regexp{
		re(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`), // session, hook and prompt ids
		re(`toolu_[0-9A-Za-z]+`), // tool call ids
	}
	if len(taskIDs) > 0 {
		quoted := make([]string, len(taskIDs))
		for i, id := range taskIDs {
			quoted[i] = regexp.QuoteMeta(id)
		}
		ids = append(ids, re(strings.Join(quoted, "|"))) // task and agent ids
	}
	return rp.Rules{
		DropKeys: []string{
			"uuid", "request_id", "timestamp", // ids and times that differ in every run
			"usage", "modelUsage", "total_cost_usd", "duration_ms", "duration_api_ms", // the model's cost: the mock has no model
			"signature",                                                                                                                 // the model's thinking, signed
			"script",                                                                                                                    // the mock's own Agent input (the sub-agent's script): the real tool has no such key
			"first_content_frame_ms", "ttft_ms", "ttft_stream_ms", "time_to_request_ms", "fast_mode_state", "fast_mode_disabled_reason", // the real service's latency and mode
		},
		// the order the capture sanitised in: the repository first, as it holds the temp root
		Scrub: []rp.Scrub{
			{Re: re(regexp.QuoteMeta(repo)), With: "<RUN>"},
			{Re: re(regexp.QuoteMeta(work)), With: "<TMP>"},
			{Re: re(regexp.QuoteMeta(enc)), With: "<RUN_DIRNAME>"},
		},
		IDs: ids,
	}
}

// unmodelled are the frames of the real stream that say nothing the mock could
// be told to say: the real run's own tools, commands and model, the account's
// rate limits, the model's thinking estimates. They are left out of both sides.
var unmodelled = map[string]bool{
	"system/init":             true, // the real run's tools, skills, slash commands and model
	"system/commands_changed": true, // the account's slash commands
	"system/thinking_tokens":  true, // the model's estimate of its own thinking
	"rate_limit_event/":       true, // the account's rate limits
}

// Frames are the stream's frames as they are compared: the unmodelled ones
// dropped, and an assistant frame without its thinking blocks (the mock has no
// model and never thinks; a frame of nothing but thinking is not compared) and
// without the API response's own bookkeeping (assistantMeta).
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
// model API's response, not to what the model said or called.
var assistantMeta = map[string]bool{
	"id": true, "model": true, "type": true, "container": true, "stop_reason": true, "stop_sequence": true,
	"stop_details": true, "context_management": true, "diagnostics": true, "input_transformations": true,
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
		content = append(content, kept)
	}
	if len(content) == 0 {
		return nil, false
	}
	message := map[string]any{}
	for k, v := range msg {
		if !assistantMeta[k] {
			message[k] = v
		}
	}
	message["content"] = content
	out := map[string]any{}
	for k, v := range f {
		out[k] = v
	}
	out["message"] = message
	return out, true
}
