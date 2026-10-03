package runner

import (
	"time"

	"github.com/sloprail/harness-mocks/internal/tasks"
)

// abortedFrame announces a background shell the harness ended, when the
// sub-agent that started it gave its final response (recorded:
// runs/foreground-subagent-bash-ends-with-response). The receipt the agent got
// did not say so; this notice follows it on the stream.
func abortedFrame(session string, t *tasks.Task) []byte {
	return jsonLine(map[string]any{
		"type": "system", "subtype": "task_notification", "task_id": t.ID, "status": "aborted",
		"title": t.Description, "session_id": session, "timestamp_ms": time.Now().UnixMilli(),
	})
}
