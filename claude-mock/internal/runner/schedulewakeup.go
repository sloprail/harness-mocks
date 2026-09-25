package runner

import (
	"encoding/json"
	"fmt"

	"github.com/sloprail/harness-mocks/claude-mock/internal/toolexec"
)

// The ScheduleWakeup tool lets the agent ask to be re-woken later. In the real
// harness it schedules a deferred resume; the mock cannot truly sleep, so calling
// the tool simply RESUMES the turn — and the runner ALREADY re-runs the scenario
// script once per tool_use, which is exactly the "wake-up fired, resume" behaviour.
// ScheduleWakeup is therefore a plain trajectory tool: it validates its args and
// returns a success tool_result; the existing turn loop does the rest. It has NO
// compaction side effect — compaction is a SEPARATE event the scenario emits as an
// isCompactSummary record (see scanLines), and the two are deliberately decoupled.
// sr:docs https://code.claude.com/docs/en/hooks#sessionstart
const toolNameScheduleWakeup = "ScheduleWakeup"

// isScheduleWakeupTool reports whether toolName is the ScheduleWakeup tool.
func isScheduleWakeupTool(toolName string) bool {
	return toolName == toolNameScheduleWakeup
}

// scheduleWakeupInput is the subset of the ScheduleWakeup tool_use input the mock
// validates. delaySeconds and prompt are required; reason is optional. These mirror
// the real tool's args — delaySeconds is the authoritative wake-up delay
// (HasPendingWakeup in the task-executor reads it back from the transcript).
type scheduleWakeupInput struct {
	DelaySeconds *int    `json:"delaySeconds"`
	Prompt       *string `json:"prompt"`
	Reason       string  `json:"reason,omitempty"`
}

// runScheduleWakeupTool handles a ScheduleWakeup tool_use. It validates the input
// args and, on success, returns a success tool_result — there is no real delay, so
// the only effect is that the turn loop re-runs the scenario script (= resume). It
// does NOT compact, and it does NOT fire SessionStart.
//
// On invalid args it returns an is_error tool_result (never crashes), matching how
// other tool errors surface.
func runScheduleWakeupTool(rawInput json.RawMessage) toolexec.Result {
	var in scheduleWakeupInput
	if err := json.Unmarshal(rawInput, &in); err != nil {
		return toolexec.Result{
			Output:  fmt.Sprintf("ScheduleWakeup: invalid tool input: %v", err),
			IsError: true,
		}
	}
	if in.DelaySeconds == nil {
		return toolexec.Result{
			Output:  "ScheduleWakeup: missing required arg \"delaySeconds\" (number of seconds to wait before waking)",
			IsError: true,
		}
	}
	if *in.DelaySeconds < 0 {
		return toolexec.Result{
			Output:  fmt.Sprintf("ScheduleWakeup: \"delaySeconds\" must be non-negative, got %d", *in.DelaySeconds),
			IsError: true,
		}
	}
	if in.Prompt == nil || *in.Prompt == "" {
		return toolexec.Result{
			Output:  "ScheduleWakeup: missing required arg \"prompt\" (the message to resume with)",
			IsError: true,
		}
	}
	// Success: text mirrors the real claude harness so the result-text fallback in
	// the task-executor's HasPendingWakeup still parses it ("Next wakeup scheduled
	// … (in Ns)…").
	return toolexec.Result{
		Output: fmt.Sprintf("Next wakeup scheduled (in %ds). The session will resume with: %s", *in.DelaySeconds, *in.Prompt),
	}
}
