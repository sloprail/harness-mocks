package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
// modelled: a wait that names a sub-agent which failed is refused, no status
// is made up for it), and timed_out. The stream shows a wait started, then
// completed with each sub-agent's state and the report of a finished one as
// its message; there is no frame of a task of its own (recorded:
// runs/foreground-subagent-result, runs/foreground-subagent-bash-ends-with-response).
// Recorded are only waits for one sub-agent that finished within the timeout:
// that a wait for several returns at the first to finish is the tool's
// description ("whichever finishes first"); a timed-out wait answers an empty
// status, as the description says ("Returns empty status when timed out").
// The sub-agents are looked up in the session's task registry: an id it does
// not hold is not_found.
//
// sr:provides foreground-subagent-result/codex
func (h toolHost) waitAgent(ctx context.Context, c toolcall.Call) toolcall.Result {
	var in waitInput
	dec := json.NewDecoder(bytes.NewReader(c.Input))
	dec.DisallowUnknownFields() // a field the real tool has not is the script's mistake, not ignored
	if err := dec.Decode(&in); err != nil {
		return toolcall.Result{Output: fmt.Sprintf("wait_agent: invalid input: %v", err), Failed: true}
	}
	item := h.events.CollabStarted("wait", h.id, in.Targets, nil)
	res, err := subagents.Wait(ctx, h.bg, in.Targets, in.timeout())
	var failed *subagents.AgentFailed
	if errors.As(err, &failed) { // fail fast: what a wait tells of a sub-agent that failed is not recorded
		return toolcall.Result{Failed: true, Output: fmt.Sprintf("codex-mock: wait_agent: %v: what Codex tells of a failed sub-agent is not recorded, so the mock refuses it rather than inventing it", failed)}
	}
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
