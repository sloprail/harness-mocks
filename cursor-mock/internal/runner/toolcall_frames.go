package runner

import "time"

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

// completedFrame is the stream frame that ends a tool call that ran: the
// call's kind and args with its result.
func completedFrame(session string, p pending, result map[string]any) []byte {
	return jsonLine(map[string]any{
		"type": "tool_call", "subtype": "completed", "call_id": p.id, "session_id": session,
		"tool_call": map[string]any{p.call.Kind: map[string]any{"args": p.call.Args, "result": result}},
	})
}

// rejectedFrame is the frame that ends a call a hook refused: the call's
// result is a rejection with the reason the agent was given.
func rejectedFrame(session string, p pending, reason string) []byte {
	rejected := map[string]any{"reason": reason, "isReadonly": false}
	if p.call.Kind == "shellToolCall" {
		rejected["command"], rejected["workingDirectory"] = p.call.Command(), ""
	} else {
		rejected["path"] = p.call.Args["path"]
	}
	return completedFrame(session, p, map[string]any{"rejected": rejected})
}
