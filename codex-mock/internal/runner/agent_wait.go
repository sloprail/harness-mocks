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
// timeout is up, and tells the agent how it stands: a status map keyed by
// sub-agent id with, for each finished one, its completion with its final report
// as text (recorded: runs/foreground-subagent-result), and timed_out. A wait for
// several returns at the first to finish, and a target still running then is not
// listed, nor is it among the receivers of the stream's completed wait item
// (recorded: runs/foreground-subagent-wait-many; the doc says Codex waits for
// all). An id that is not a sub-agent of the session's task registry is
// "not_found" (the tool description's status; no recording shows it); a timed-out
// wait answers an empty status, as the description says. What a wait tells of a
// sub-agent that failed is not recorded: it is refused, not made up. The stream
// shows a wait started, then completed; there is no frame of a task of its own.
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
	var status, told []string
	for _, st := range res.States {
		switch st.Status {
		case subagents.WaitCompleted:
			// a sub-agent that ended with no message is completed with null (recorded: runs/subagent-stop-no-message)
			var msg any = st.Report
			completed := fmt.Sprintf("%q", st.Report)
			if st.Report == "" {
				msg, completed = nil, "null"
			}
			states[st.ID] = events.AgentState{Status: "completed", Message: msg}
			status = append(status, fmt.Sprintf("%q:{\"completed\":%s}", st.ID, completed))
		case subagents.WaitNotFound:
			status = append(status, fmt.Sprintf("%q:%q", st.ID, st.Status))
		default: // still running: a wait that returned at the first to finish does not list it (recorded: runs/foreground-subagent-wait-many)
			continue
		}
		told = append(told, st.ID)
	}
	h.events.CollabCompleted(item, "wait", h.id, told, nil, states)
	return toolcall.Result{Output: fmt.Sprintf(`{"status":{%s},"timed_out":%t}`, strings.Join(status, ","), res.TimedOut)}
}
