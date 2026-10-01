package runner

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/sloprail/harness-mocks/claude-mock/internal/toolexec"
	"github.com/sloprail/harness-mocks/internal/tools"
)

// The ScheduleWakeup tool lets the agent ask to be re-woken later (the pacing of
// a self-paced /loop). In the real harness it schedules a deferred resume; the
// mock cannot truly sleep, so calling the tool acknowledges the request and the
// turn goes on, which the runner already does by re-running the scenario script
// once per tool_use: that is the "wake-up fired, resume" behaviour. It has NO
// compaction side effect — compaction is a SEPARATE event the scenario emits.
// What a request amounts to (the clamped delay, the minute it is set for, the one
// pending wake-up, stop) is the tools core's (tools.Wakeups); this is Claude
// Code's tool: its input, its texts and its result.
// sr:docs https://code.claude.com/docs/en/tools-reference#tool-behavior
const toolNameScheduleWakeup = "ScheduleWakeup"

// isScheduleWakeupTool reports whether toolName is the ScheduleWakeup tool.
func isScheduleWakeupTool(toolName string) bool {
	return toolName == toolNameScheduleWakeup
}

// scheduleWakeupInput is a ScheduleWakeup tool_use's input: delaySeconds and
// prompt (the delay and what to resume with), reason, noop (required unless
// stop) and stop (cancel the pending wake-up).
type scheduleWakeupInput struct {
	DelaySeconds *int    `json:"delaySeconds"`
	Prompt       *string `json:"prompt"`
	Reason       string  `json:"reason,omitempty"`
	Noop         *bool   `json:"noop"`
	Stop         bool    `json:"stop"`
}

// runScheduleWakeupTool handles a ScheduleWakeup tool_use, with the texts and the
// result Claude Code 2.1.285 gave (recorded: snapshots/runs/schedule-wakeup,
// schedule-wakeup-limits). An invalid call is an is_error tool_result, never a
// crash.
//
// sr:provides schedule-wakeup/claude
func runScheduleWakeupTool(w *tools.Wakeups, rawInput json.RawMessage) toolexec.Result {
	var in scheduleWakeupInput
	if err := json.Unmarshal(rawInput, &in); err != nil {
		return toolexec.Result{Output: fmt.Sprintf("ScheduleWakeup: invalid tool input: %v", err), IsError: true}
	}
	now := time.Now()
	res, err := w.Schedule(tools.WakeupRequest{
		DelaySeconds: in.DelaySeconds, Prompt: in.Prompt, Reason: in.Reason, Noop: in.Noop, Stop: in.Stop,
	}, now, func() string { return randomID(8) })
	var missing *tools.MissingArgError
	switch {
	case errors.As(err, &missing) && missing.Arg == "noop":
		return toolexec.Result{Output: "`noop` is required when `stop` is not true.", IsError: true}
	case errors.As(err, &missing) && missing.Arg == "delaySeconds":
		return toolexec.Result{Output: "ScheduleWakeup: missing required arg \"delaySeconds\" (number of seconds to wait before waking)", IsError: true}
	case missing != nil:
		return toolexec.Result{Output: "ScheduleWakeup: missing required arg \"prompt\" (the message to resume with)", IsError: true}
	}
	if res.Stopped {
		return toolexec.Result{
			Output: fmt.Sprintf("Loop stopped — cancelled %d pending wakeup(s); no further dynamic-loop wakeups scheduled. If you armed a Monitor for this loop, TaskStop it now. Then write the loop's outcome for the user as ordinary visible response text — and end the turn.", res.Cancelled),
			ToolUseResult: map[string]any{
				"scheduledFor": 0, "clampedDelaySeconds": 0, "wasClamped": false, "stopped": true, "cancelledWakeups": res.Cancelled,
			},
		}
	}
	clamped := ""
	if res.Clamped {
		clamped = fmt.Sprintf(" (clamped to %ds from your requested value)", res.DelaySeconds)
	}
	return toolexec.Result{
		Output: fmt.Sprintf("Next wakeup scheduled for %s (in %ds)%s. If you owe the user a status update this tick, write it now as ordinary response text; then end the turn — the harness re-invokes you when the wakeup fires or a task-notification arrives.",
			res.At.Local().Format("15:04:05"), res.SecondsUntil(now), clamped),
		ToolUseResult: map[string]any{
			"scheduledFor": res.At.UnixMilli(), "clampedDelaySeconds": res.DelaySeconds, "wasClamped": res.Clamped,
		},
	}
}

// sessionCrons is the pending wake-up as a Stop payload's session_crons lists it:
// {id, schedule: a cron line of its minute, recurring: false, prompt}.
func sessionCrons(w *tools.Wakeups) []any {
	crons := []any{}
	for _, p := range w.Pending() {
		at := p.At.Local()
		crons = append(crons, map[string]any{
			"id": p.ID, "schedule": fmt.Sprintf("%d %d * * *", at.Minute(), at.Hour()), "recurring": false, "prompt": p.Prompt,
		})
	}
	return crons
}
