package runner

import (
	"encoding/json"
	"fmt"

	"github.com/sloprail/harness-mocks/internal/subagents"
	"github.com/sloprail/harness-mocks/internal/toolcall"
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

// moreFollows is whether the call is one of several made by one script of the model
// (the mock's own parameter `more`): the model is not asked again until the last,
// so a sub-agent's end is not told of in between (recorded: runs/foreground-subagent-wait-many).
func moreFollows(c toolcall.Call) bool {
	var in struct {
		More bool `json:"more"`
	}
	_ = json.Unmarshal(c.Input, &in)
	return in.More
}

// afterOutput is what follows the output a call was given: the call has finished (what another
// agent's gate may wait for), and when the model is asked again (not between the calls of
// one script) it is told of a sub-agent that has ended, one at a time.
func (h toolHost) afterOutput(c toolcall.Call) {
	h.prog.move(0, 1)
	if moreFollows(c) {
		return
	}
	if text, ok := h.nextNotice(); ok {
		h.rollout.User(text)
	}
}
