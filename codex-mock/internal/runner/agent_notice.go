package runner

import (
	"encoding/json"

	"github.com/sloprail/harness-mocks/internal/tasks"
)

// tellFinishedSubAgents puts into the session's rollout, as a message of the
// user's, what the harness tells an agent once a sub-agent it started has ended:
// <subagent_notification> around the sub-agent's id and its final
// answer. It comes after the tool output the agent was just
// given, once, whether or not the agent had waited for the sub-agent (recorded:
// runs/subagent-transcripts-v2: after the wait's output, before the next answer).
func (h toolHost) tellFinishedSubAgents() {
	for _, t := range h.bg.TakeFinished(h.id) {
		if t.Kind != tasks.Agent {
			continue
		}
		if t.Failure != "" { // what the harness tells of a sub-agent that failed is not recorded: the wait refuses it
			continue
		}
		b, _ := json.Marshal(struct {
			AgentPath string            `json:"agent_path"`
			Status    map[string]string `json:"status"`
		}{t.ID, map[string]string{"completed": t.Result}})
		h.rollout.User("<subagent_notification>\n" + string(b) + "\n</subagent_notification>")
	}
}
