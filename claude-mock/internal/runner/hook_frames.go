package runner

import "github.com/sloprail/harness-mocks/claude-mock/internal/hooks"

// The stream frames a stream-json run carries for hooks it fired. Only these
// write them.

// writeSessionStartFrames streams each SessionStart handler's run as a
// hook_started frame and a hook_response frame carrying its output, exit code
// and outcome ("success" on exit 0, else "error"), ahead of everything else
// the session streams (recorded: snapshots/runs/hook-exit-codes,
// subprocess-session-env). The handlers have already run; the frames keep the
// recorded order.
func writeSessionStartFrames(cfg Config, in hooks.Input, runs []hooks.HandlerRun) {
	name := hookRunName(in)
	ids := make([]string, len(runs))
	for i := range runs {
		ids[i] = newRecordUUID()
		writeFrame(cfg, map[string]any{
			"type": "system", "subtype": "hook_started", "hook_id": ids[i],
			"hook_name": name, "hook_event": string(in.HookEventName),
		})
	}
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

// writeStopHookError streams the notice a Stop hook that blocked (exit 2) or
// failed (a non-blocking error) leaves: one notification per firing, whatever
// the number of failing handlers (recorded: snapshots/runs/hook-exit-codes,
// exit 2; hook-exit-json, exit 1). A Stop whose handlers all succeeded leaves
// none.
func writeStopHookError(cfg Config, runs []hooks.HandlerRun) {
	for _, r := range runs {
		if r.Blocked || r.JSONError != "" || (r.ExitCode != 0 && !r.JSONParsed) {
			writeFrame(cfg, map[string]any{
				"type": "system", "subtype": "notification", "key": "stop-hook-error",
				"text": "Stop hook error occurred · ctrl+o to see", "priority": "immediate",
			})
			return
		}
	}
}
