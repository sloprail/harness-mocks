package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sloprail/harness-mocks/codex-mock/internal/events"
	"github.com/sloprail/harness-mocks/internal/subagents"
	"github.com/sloprail/harness-mocks/internal/toolcall"
)

// waitInput is a wait_agent call: the sub-agents to wait for, and how long
// (timeout_ms; the tool's own description: default 30000, at least 10000, at
// most 3600000).
type waitInput struct {
	Targets []string `json:"targets"`
	Timeout *int     `json:"timeout_ms"`
}

func (in waitInput) timeout() time.Duration {
	return subagents.Timeout(in.Timeout, 30000, 10000, 3600000)
}

// waitAgent waits until one of the sub-agents it names has finished, or its
// timeout is up, and tells the agent where each stands: a status map keyed by
// sub-agent id, a finished one's value its completion with its final report as
// text, one still going "running", one that is not a sub-agent of the session
// "not_found" (the last two are the tool description's statuses, which no
// recording shows; recorded is only {completed} with timed_out false; the
// description's pending_init, interrupted, shutdown and errored are not
// modelled), and timed_out. The stream shows a wait started, then
// completed with each sub-agent's state and the report of a finished one as
// its message; there is no frame of a task of its own (recorded:
// runs/foreground-subagent-result, runs/foreground-subagent-bash-ends-with-response).
// Recorded are only waits for one sub-agent that finished within the timeout:
// that a wait for several returns at the first to finish is the tool's
// description ("whichever finishes first"); a timed-out wait answers an empty
// status, as the description says ("Returns empty status when timed out").
// One run per process: the sub-agents are looked up in the process's table
// (agents), so an id of another run is not_found only by being absent.
//
// sr:provides foreground-subagent-result/codex
func (h toolHost) waitAgent(ctx context.Context, c toolcall.Call) toolcall.Result {
	var in waitInput
	_ = json.Unmarshal(c.Input, &in)
	item := h.events.CollabStarted("wait", h.id, in.Targets, nil)
	res, err := subagents.Wait(ctx, h.bg, in.Targets, in.timeout())
	if err != nil {
		return toolcall.Result{Output: "wait interrupted", Failed: true}
	}
	states := map[string]events.AgentState{}
	var status []string
	for _, st := range res.States {
		switch st.Status {
		case subagents.WaitCompleted:
			states[st.ID] = events.AgentState{Status: "completed", Message: st.Report}
			status = append(status, fmt.Sprintf("%q:{\"completed\":%q}", st.ID, st.Report))
		default:
			status = append(status, fmt.Sprintf("%q:%q", st.ID, st.Status))
		}
	}
	h.events.CollabCompleted(item, "wait", h.id, in.Targets, nil, states)
	return toolcall.Result{Output: fmt.Sprintf(`{"status":{%s},"timed_out":%t}`, strings.Join(status, ","), res.TimedOut)}
}
