package runner

import (
	"context"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
)

// The stream frames a stream-json run carries for hooks it fired. Only these
// write them.

// writeSessionStartFrames streams each SessionStart handler's run as a
// hook_started frame and a hook_response frame carrying its output, exit code
// and outcome ("success" on exit 0, else "error"), ahead of everything else
// the session streams (recorded: snapshots/runs/hook-exit-codes,
// subprocess-session-env). The handlers have already run; the frames keep the
// recorded order.
func writeSessionStartFrames(cfg Config, in hooks.Input, runs []hooks.HandlerRun) {
	if cfg.ResumeLookup && in.Source == startSource(corehooks.StartResumed) {
		cfg.SessionID = newRecordUUID() // a session found by name, path or --continue: its hooks' frames carry the id of the lookup's own session (recorded: runs/resume-name)
	}
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

// startSource is SessionStart's source for how a session began.
//
// sr:provides session-start-hook/claude
func startSource(kind corehooks.StartKind) string {
	switch kind {
	case corehooks.StartResumed:
		return "resume"
	case corehooks.StartForked:
		return "fork"
	case corehooks.StartCompacted:
		return "compact"
	default:
		return "startup"
	}
}

// endReason is SessionEnd's reason for why a session ended. The interactive
// reasons are not modeled (adr/modeled-surface).
//
// sr:provides session-end-hook/claude
func endReason(corehooks.EndReason) string { return "other" }

// submitPrompt fires UserPromptSubmit for a prompt, if the prompt is one the
// hooks see, and returns the context the hooks added. refused reports a hook
// blocked it: the run ends with err, what promptBlocked leaves behind.
//
// sr:provides user-prompt-submit-hook/claude
func submitPrompt(ctx context.Context, cfg Config, inv *hooks.Invoker, tr *transcript, src corehooks.PromptSource, held bool) (extra string, refused bool, err error) {
	if cfg.Prompt == "" || !corehooks.PromptHookFires(src) {
		return "", false, nil
	}
	if held { // what the hooks leave follows the prompt once it is written
		inv = inv.WithRecorder(tr.holdHookRuns)
	}
	in := hooks.Input{SessionID: cfg.SessionID, Cwd: cfg.Cwd, HookEventName: hooks.EventUserPromptSubmit, Prompt: cfg.Prompt}
	out, runs, ferr := inv.FireRuns(ctx, in)
	writeHookEventFrames(cfg, in, runs)
	refused, extra = corehooks.PromptOutcome(ferr != nil || out.Decision == "block", promptContextFrom(out))
	if refused {
		if ferr == nil { // an exit-0 hook that blocked by its JSON decision (recorded: snapshots/runs/prompt-blocked-json)
			ferr = &hooks.BlockError{Reason: out.Reason, SuppressPrompt: out.HookSpecificOutput != nil && out.HookSpecificOutput.SuppressOriginalPrompt}
		}
		if held {
			tr.dropHeldHookRuns() // a refused prompt leaves only its warning
		}
		return "", true, promptBlocked(cfg, tr, ferr)
	}
	return extra, false, nil
}

// fireCompactedStart fires SessionStart for the session continuing after a
// compaction. Its context is not carried to a next turn here, and no hook
// stops it.
//
// sr:provides session-start-hook/claude
func fireCompactedStart(ctx context.Context, cfg Config, inv *hooks.Invoker) {
	_, _ = fireSessionStart(ctx, cfg, inv, corehooks.SessionStartKind(false, false, true))
}

// Prompting is what hook payloads tell of the prompt a session is on.
type Prompting struct {
	// PermissionMode is the permission_mode the hooks about a turn are told.
	// sr:docs https://code.claude.com/docs/en/hooks#common-input-fields
	PermissionMode string
	// MaxTurns is --max-turns: the model turns a run may take (0: no limit).
	MaxTurns int
	// HookEvents is --include-hook-events: the stream carries a hook_started and a
	// hook_response frame for each hook of the main thread, not only SessionStart's.
	HookEvents bool
	// Turn is the prompt the session is on: the root run makes it, every
	// sub-agent run inside shares it.
	Turn *hooks.Turn
}
