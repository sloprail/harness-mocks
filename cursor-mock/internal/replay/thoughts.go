package replay

import (
	core "github.com/sloprail/harness-mocks/internal/replay"
)

// thoughtKeys are the keys of an afterAgentThought payload the adapter reads: the
// thinking text, and what it says of the model that thought (the mock puts the
// latter back in the payload from the script). The rest are what every payload
// carries or what differs in every run (the generation, how long it took).
var (
	thoughtModel  = map[string]bool{"model": true, "model_id": true, "model_params": true}
	thoughtCommon = map[string]bool{
		"text": true, "conversation_id": true, "generation_id": true, "duration_ms": true, "session_id": true,
		"hook_event_name": true, "cursor_version": true, "workspace_roots": true, "user_email": true, "transcript_path": true,
	}
)

// thoughtsOf are the thoughts the recording's hook log holds, by the conversation
// that had them, in the order they were had. A key of a payload that is neither
// one the adapter reads nor one every payload carries is something of the
// recording it cannot reproduce.
func thoughtsOf(payloads []map[string]any) (map[string][]*core.Thinking, error) {
	out := map[string][]*core.Thinking{}
	for _, p := range payloads {
		if p["hook_event_name"] != "afterAgentThought" {
			continue
		}
		th := &core.Thinking{Fields: map[string]any{}}
		th.Text, _ = p["text"].(string)
		for k, v := range p {
			switch {
			case thoughtModel[k]:
				th.Fields[k] = v
			case !thoughtCommon[k]:
				return nil, unbuildable("an afterAgentThought payload holds %q, which the adapter does not read", k)
			}
		}
		id, _ := p["session_id"].(string)
		out[id] = append(out[id], th)
	}
	return out, nil
}
