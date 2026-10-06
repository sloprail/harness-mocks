package replay

import (
	"strings"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// toolReply is the unified answer (core.ToolAnswer) the model gave that a Stop hook then refused
// to end the turn on: Input "text". It is not a tool call: the model said it, and was told to go on.
const toolReply = core.ToolAnswer

// isNotificationTurn is whether a user record is a background task's notification handed over as a turn.
func isNotificationTurn(rec map[string]any) bool {
	msg, _ := rec["message"].(map[string]any)
	s, _ := msg["content"].(string)
	return strings.HasPrefix(s, "<task-notification>")
}

// stopFeedback starts the user record a blocking Stop hook leaves.
const stopFeedback = "Stop hook feedback:"

// isStopFeedback is whether a user record is a Stop hook's feedback.
func isStopFeedback(rec map[string]any) bool {
	msg, _ := rec["message"].(map[string]any)
	s, _ := msg["content"].(string)
	return strings.HasPrefix(s, stopFeedback)
}

// toolSend is a message to a sub-agent that goes on from where it stopped: Input is the call's own
// (to, message, type, recipient, content), with to and recipient the id of the agent the harness minted.
const toolSend = "send_message"
