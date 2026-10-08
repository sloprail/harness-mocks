package runner

import (
	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
	"github.com/sloprail/harness-mocks/internal/tasks"
)

// writeHookEventFrames streams the frames of a hook a main-thread event fired
// when --include-hook-events asks for them (recorded: snapshots/runs/include-hook-events:
// UserPromptSubmit, PreToolUse:<tool>, PostToolUse:<tool> and Stop each stream a
// started and a response frame, in firing order). SessionStart's are always streamed.
func writeHookEventFrames(cfg Config, in hooks.Input, runs []hooks.HandlerRun) {
	if cfg.HookEvents && cfg.AgentID == "" {
		writeSessionStartFrames(cfg, in, runs)
	}
}

// writeSessionStartFrames streams each handler's run as a hook_started frame and a
// hook_response frame carrying its output, exit code and outcome ("success" on exit 0, else
// "error"): SessionStart's ahead of everything else the session streams (recorded:
// snapshots/runs/hook-exit-codes, subprocess-session-env). The handlers have already run; the
// frames keep the recorded order.
func writeSessionStartFrames(cfg Config, in hooks.Input, runs []hooks.HandlerRun) {
	if cfg.ResumeLookup && in.Source == startSource(corehooks.StartResumed) {
		cfg.SessionID = newRecordUUID() // a session found by name, path or --continue: its hooks' frames carry the id of the lookup's own session (recorded: runs/resume-name)
	}
	writeHookResponses(cfg, in, runs, writeHookStarted(cfg, in, runs))
}

// writeHookStarted streams the hook_started frame of each handler's run, and returns the ids of
// the runs: a SubagentStart's response frames come after the sub-agent's first frame (recorded:
// runs/include-hook-events-more).
func writeHookStarted(cfg Config, in hooks.Input, runs []hooks.HandlerRun) []string {
	name := hookRunName(in)
	ids := make([]string, len(runs))
	for i := range runs {
		ids[i] = newRecordUUID()
		writeFrame(cfg, map[string]any{
			"type": "system", "subtype": "hook_started", "hook_id": ids[i],
			"hook_name": name, "hook_event": string(in.HookEventName),
		})
	}
	return ids
}

// writeHookResponses streams the hook_response frame of each run started with ids.
func writeHookResponses(cfg Config, in hooks.Input, runs []hooks.HandlerRun, ids []string) {
	name := hookRunName(in)
	for i, r := range runs {
		outcome := "success"
		if r.ExitCode != 0 {
			outcome = "error"
		}
		writeFrame(cfg, map[string]any{
			"type": "system", "subtype": "hook_response", "hook_id": ids[i],
			"hook_name": name, "hook_event": string(in.HookEventName),
			"output": r.Stdout + r.Stderr, "stdout": r.Stdout, "stderr": r.Stderr,
			"exit_code": r.ExitCode, "outcome": outcome,
		})
	}
}

// pendingFrames are the frames of a hook whose response comes after the frame that follows its
// start: SubagentStart's hook_started frame is streamed when the hook runs, the sub-agent's first
// frame (its prompt) next, and its hook_response frame after that (recorded: include-hook-events-more).
type pendingFrames struct {
	in   hooks.Input
	runs []hooks.HandlerRun
	ids  []string
}

// startPending streams the hook_started frames of a main-thread event's hook and holds the rest;
// nil when --include-hook-events is not asked for or the event is not the main thread's.
func startPending(cfg Config, in hooks.Input, runs []hooks.HandlerRun) *pendingFrames {
	if !cfg.HookEvents || cfg.AgentID != "" {
		return nil
	}
	return &pendingFrames{in: in, runs: runs, ids: writeHookStarted(cfg, in, runs)}
}

// finish streams the response frames held, once.
func (p *pendingFrames) finish(cfg Config) {
	if p != nil && p.runs != nil {
		writeHookResponses(cfg, p.in, p.runs, p.ids)
		p.runs = nil
	}
}

// refuseBackgroundHookFrames refuses the launch of a background task when --include-hook-events is
// asked for and the task would fire a hook whose frames are not recorded (adr/fail-fast-unimplemented):
// its notification starts a turn through UserPromptSubmit, and a background sub-agent's SubagentStart and
// SubagentStop come with it (only a foreground sub-agent's frames are recorded).
func refuseBackgroundHookFrames(cfg Config, inv *hooks.Invoker, call pendingToolUse) error {
	if !cfg.HookEvents || cfg.AgentID != "" {
		return nil
	}
	agent := isAgentTool(call.ToolName) && agentRunsInBackground(cfg, call.ToolInput)
	bash := call.ToolName == "Bash" && tasks.RunsInBackground(runsInBackground(call.ToolInput), cfg.BackgroundTasksDisabled)
	switch {
	case agent:
		return refuseUnrecordedHook(cfg, inv, hooks.EventSubagentStart, hooks.EventSubagentStop, hooks.EventUserPromptSubmit)
	case bash:
		return refuseUnrecordedHook(cfg, inv, hooks.EventUserPromptSubmit)
	}
	return nil
}
