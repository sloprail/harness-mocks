package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sloprail/harness-mocks/codex-mock/internal/events"
	"github.com/sloprail/harness-mocks/internal/tasks"
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
	ms := 30000
	if in.Timeout != nil {
		ms = min(max(*in.Timeout, 10000), 3600000)
	}
	return time.Duration(ms) * time.Millisecond
}

// waitAgent waits until one of the sub-agents it names has finished, or its
// timeout is up, and tells the agent where each stands: a status map keyed by
// sub-agent id, a finished one's value its completion with its final report as
// text, one still going "running", one that is not a sub-agent of the session
// "not_found" (the statuses the tool's own description lists, recorded in every
// run that calls it), and timed_out. The stream shows a wait started, then
// completed with each sub-agent's state and the report of a finished one as
// its message; there is no frame of a task of its own (recorded:
// runs/foreground-subagent-result, runs/foreground-subagent-bash-ends-with-response).
// Recorded are only waits for one sub-agent that finished within the timeout:
// that a wait for several returns at the first to finish is the tool's
// description ("whichever finishes first"), and what a timed-out wait tells is
// the mock's reading of the same description.
//
// sr:provides foreground-subagent-result/codex
func (h toolHost) waitAgent(ctx context.Context, c toolcall.Call) toolcall.Result {
	var in waitInput
	_ = json.Unmarshal(c.Input, &in)
	item := h.events.CollabStarted("wait", h.id, in.Targets, nil)

	var known []*tasks.Task
	for _, id := range in.Targets {
		if v, ok := agentTasks.Load(id); ok {
			known = append(known, v.(*tasks.Task))
		}
	}
	timedOut := len(known) > 0
	if len(known) == 0 {
		timedOut = false
	} else {
		timer := time.NewTimer(in.timeout())
		defer timer.Stop()
		for waiting := true; waiting; {
			for _, t := range known {
				if t.Finished() {
					waiting, timedOut = false, false
				}
			}
			if !waiting {
				break
			}
			select {
			case <-timer.C:
				waiting = false
			case <-ctx.Done():
				return toolcall.Result{Output: "wait interrupted", Failed: true}
			case <-time.After(20 * time.Millisecond):
			}
		}
	}

	states := map[string]events.AgentState{}
	var status []string
	for _, id := range in.Targets {
		v, ok := agentTasks.Load(id)
		switch t, _ := v.(*tasks.Task); {
		case !ok:
			status = append(status, fmt.Sprintf("%q:%q", id, "not_found"))
		case t.Finished():
			states[id] = events.AgentState{Status: "completed", Message: t.Result}
			status = append(status, fmt.Sprintf("%q:{\"completed\":%q}", id, t.Result))
		default:
			status = append(status, fmt.Sprintf("%q:%q", id, "running"))
		}
	}
	h.events.CollabCompleted(item, "wait", h.id, in.Targets, nil, states)
	return toolcall.Result{Output: fmt.Sprintf(`{"status":{%s},"timed_out":%t}`, strings.Join(status, ","), timedOut)}
}
