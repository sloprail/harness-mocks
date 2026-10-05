package runner

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/sloprail/harness-mocks/internal/subagents"
	"github.com/sloprail/harness-mocks/internal/toolcall"
)

// The mock has no model to take the time a model call takes, so two durations stand for it:
// modelCall is how long a sub-agent takes to start (its first step is a model call: recorded,
// the agent that spawned it makes its next call first, runs/background-agent), and
// answerLatency how long the end of a turn gives a running sub-agent to end and be told of
// before the turn does (the model's last answer takes a while: runs/foreground-subagent-wait-many,
// where the second sub-agent ended while the model answered). A sub-agent that runs on past it
// does not hold the turn up (runs/print-waits-for-background-agents).
const (
	modelCall     = time.Second
	answerLatency = 4 * time.Second
)

// nextNotice is the <subagent_notification> text the harness tells the agent of
// the next sub-agent it started that has ended: around the sub-agent's id and its
// final answer (recorded: runs/subagent-transcripts-v2, runs/foreground-subagent-wait-many).
// Which sub-agent, and when, is the core's (subagents.TakeNotice); this is the
// encoding. What the harness tells of one that failed is not recorded, so a failed
// one is reported on stderr and no message is made up for it. ok is false when
// there is none to tell of. await is whether to give a running sub-agent a moment
// to end first (answerLatency): at the end of a turn, not after a tool output.
//
// sr:provides subagent-transcripts/codex
func (h toolHost) nextNotice(await bool) (text string, ok bool) {
	for {
		n, found := subagents.TakeNotice(h.bg, h.id)
		if !found && await {
			n, found = subagents.AwaitNotice(h.bg, h.id, answerLatency)
		}
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
