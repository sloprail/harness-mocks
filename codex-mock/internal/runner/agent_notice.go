package runner

import (
	"encoding/json"
	"fmt"

	"github.com/sloprail/harness-mocks/internal/subagents"
)

// nextNotice is the <subagent_notification> text the harness tells the agent of
// the next sub-agent it started that has ended: around the sub-agent's id and its
// final answer (recorded: runs/subagent-transcripts-v2, runs/foreground-subagent-wait-many).
// Which sub-agent, and when, is the core's (subagents.TakeNotice); this is the
// encoding. What the harness tells of one that failed is not recorded, so a failed
// one is reported on stderr and no message is made up for it. ok is false when
// there is none to tell of: what is told is what has ended, and when a sub-agent ends
// relative to the agent's steps is the script's (scenario.Gate), not a delay's.
//
// sr:provides subagent-transcripts/codex
func (h toolHost) nextNotice() (text string, ok bool) {
	for {
		n, found := subagents.TakeNotice(h.bg, h.id)
		if !found {
			return "", false
		}
		if n.Failure != "" {
			fmt.Fprintf(h.cfg.Stderr, "ERROR codex_mock: sub-agent %s ended without an answer: %s\n", n.ID, n.Failure)
			continue
		}
		b, _ := json.Marshal(struct {
			AgentPath string            `json:"agent_path"`
			Status    map[string]string `json:"status"`
		}{n.ID, map[string]string{"completed": n.Answer}})
		return "<subagent_notification>\n" + string(b) + "\n</subagent_notification>", true
	}
}
