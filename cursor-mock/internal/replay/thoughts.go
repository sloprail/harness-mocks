package replay

import (
	"strconv"
	"strings"

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
// that had them and the response that had each. A key of a payload that is neither
// one the adapter reads nor one every payload carries is something of the
// recording it cannot reproduce.
//
// A conversation is several model requests when the harness gave the agent more
// than one turn (a finished background task starts a new one), and each request
// numbers its responses from 0: a thought is placed by the order its request was
// first heard of, and its number within it (recorded: runs/background-bash-start).
func thoughtsOf(payloads []map[string]any) (map[string][]thoughtAt, error) {
	out := map[string][]thoughtAt{}
	requests := map[string][]string{} // per conversation, the requests in the order a thought named them
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
		// the thought names the response that had it: its generation is the model
		// call, <request>-<response number>-<four characters>
		n, err := responseOf(p["generation_id"])
		if err != nil {
			return nil, err
		}
		req := requestOf(p["generation_id"])
		ri := -1
		for i, r := range requests[id] {
			if r == req {
				ri = i
			}
		}
		if ri < 0 {
			ri = len(requests[id])
			requests[id] = append(requests[id], req)
		}
		out[id] = append(out[id], thoughtAt{request: ri, response: n, thinking: th})
	}
	return out, nil
}

// responseOf is the number of the model response a thought's generation names.
func responseOf(generation any) (int, error) {
	g, _ := generation.(string)
	parts := strings.Split(g, "-")
	if len(parts) < 2 {
		return 0, unbuildable("a thought's generation %q does not name its response", g)
	}
	n, err := strconv.Atoi(parts[len(parts)-2])
	if err != nil {
		return 0, unbuildable("a thought's generation %q does not name its response", g)
	}
	return n, nil
}

// thoughtAt is a thought and the response that had it: its number within the
// request, and the request's place among the conversation's requests.
type thoughtAt struct {
	request, response int
	thinking          *core.Thinking
}

// requestOf is the model request a thought's generation names: all of it but
// the response number and the four characters.
func requestOf(generation any) string {
	g, _ := generation.(string)
	parts := strings.Split(g, "-")
	if len(parts) < 2 {
		return g
	}
	return strings.Join(parts[:len(parts)-2], "-")
}
