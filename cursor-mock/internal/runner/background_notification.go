package runner

import (
	"time"

	"github.com/sloprail/harness-mocks/internal/tasks"
)

// followUp is the prompt of the turn Cursor starts for a finished sub-agent
// (recorded: runs/print-waits-for-background-agents).
const followUp = "Perform any necessary follow-up actions in response to the subagent completion above. If no follow-up work is needed, no further action is required. If you mention an agent or subagent in your response, link it with the `[Name](id)` Don't use generic label such as `[agent]`, `[worker]`, or `[subagent]`. Don't repeat the same confirmation every time."

// notification is what a finished background task puts on the stream, and the
// prompt of the further turn it starts. A print session stays open while a
// background sub-agent runs (afterTurn); its end is announced with its prompt,
// cut to 80 characters, as the title and what it said as the detail.
//
// sr:provides print-waits-for-background-agents/cursor
func (s *session) notification(t *tasks.Task) (frame []byte, prompt string) {
	if t.Kind != tasks.Agent {
		return notificationFrame(s.id, t), notificationPrompt
	}
	title, _ := t.Meta.(string)
	if r := []rune(title); len(r) > 80 {
		title = string(r[:80])
	}
	status, detail := "success", t.Result
	if t.Status() != tasks.Completed {
		status, detail = "error", t.Failure
	}
	return jsonLine(map[string]any{
		"type": "system", "subtype": "task_notification", "task_id": t.ID, "status": status,
		"title": title, "detail": detail, "session_id": s.id, "timestamp_ms": time.Now().UnixMilli(),
	}), followUp
}
