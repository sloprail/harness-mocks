package runner

import (
	"encoding/json"
	"fmt"

	"github.com/sloprail/harness-mocks/internal/subagents"
)

// tellFinishedSubAgents puts into the session's rollout, as a message of the
// user's, what the harness tells an agent once a sub-agent it started has ended:
// <subagent_notification> around the sub-agent's id and its final answer (recorded:
// runs/subagent-transcripts-v2: after the wait's output, before the next answer).
// Which sub-agents, and when, is the core's (subagents.TakeNotices); this is the
// encoding. What the harness tells of one that failed is not recorded, so a failed
// one is reported on stderr and no message is made up for it.
//
// sr:provides subagent-transcripts/codex
func (h toolHost) tellFinishedSubAgents() {
	for _, n := range subagents.TakeNotices(h.bg, h.id) {
		if n.Failure != "" {
			fmt.Fprintf(h.cfg.Stderr, "ERROR codex_mock: sub-agent %s ended without an answer: %s\n", n.ID, n.Failure)
			continue
		}
		b, _ := json.Marshal(struct {
			AgentPath string            `json:"agent_path"`
			Status    map[string]string `json:"status"`
		}{n.ID, map[string]string{"completed": n.Answer}})
		h.rollout.User("<subagent_notification>\n" + string(b) + "\n</subagent_notification>")
	}
}
