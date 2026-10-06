package runner

import (
	"context"
	"path/filepath"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
	"github.com/sloprail/harness-mocks/claude-mock/internal/toolexec"
	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
)

// The stream frames a stream-json run carries for hooks it fired. Only these
// write them.

// writeStopHookFrames streams what a Stop hook leaves in the stream: a blocked turn's feedback to the agent,
// then the notice a Stop hook that blocked (by exit 2 or by
// JSON: blocked says the turn was held on) or failed (a non-blocking error) leaves:
// one notification per firing, whatever the number of failing handlers (recorded:
// snapshots/runs/hook-exit-codes, exit 2; hook-exit-json, exit 1; stops, JSON). A Stop whose
// handlers all succeeded leaves none.
//
// The notice is shown once per run: a later Stop that blocks or fails leaves none (recorded:
// snapshots/runs/stops, cap), and shown reports whether it was already.
func writeStopHookFrames(cfg Config, runs []hooks.HandlerRun, blocked bool, shown *bool) {
	if blocked {
		writeStopFeedbackFrames(cfg, runs)
	}
	if *shown {
		return
	}
	for _, r := range runs {
		if blocked || r.Blocked || r.JSONError != "" || (r.ExitCode != 0 && !r.JSONParsed) {
			writeFrame(cfg, map[string]any{
				"type": "system", "subtype": "notification", "key": "stop-hook-error",
				"text": "Stop hook error occurred · ctrl+o to see", "priority": "immediate",
			})
			*shown = true
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
	// Scratchpad: the session has a scratchpad directory, which the hooks are told of.
	Scratchpad bool
	// Turn is the prompt the session is on: the root run makes it, every
	// sub-agent run inside shares it.
	Turn *hooks.Turn
}

// configureInvoker gives the invoker what the payloads of this session carry besides what the
// event brings: its permission mode and, when it has one, its scratchpad (<session dir>/scratchpad,
// beside the tasks: recorded in snapshots/runs/nested-session-env).
func (cfg Config) configureInvoker(inv *hooks.Invoker) {
	inv.SetPermissionMode(cfg.PermissionMode)
	inv.SetConfigDir(cfg.ConfigDir)
	toolexec.SetConfigDir(cfg.ConfigDir) // the Bash commands source the env files its SessionStart hooks write
	if cfg.Scratchpad {
		inv.SetScratchpadDir(filepath.Join(filepath.Dir(tasksDir(cfg.Cwd, cfg.SessionID)), "scratchpad"))
	}
}

// writeStopFeedbackFrames streams what a Stop hook that blocks hands the agent: a synthetic user
// message carrying the feedback, one per handler that gave some (recorded: snapshots/runs/stops, cap,
// hook-exit-codes), ahead of the notice of the error.
func writeStopFeedbackFrames(cfg Config, runs []hooks.HandlerRun) {
	for _, r := range runs {
		text, ok := stopFeedbackOf(hooks.EventStop, r)
		if !ok {
			continue
		}
		line, err := marshalRecord(map[string]any{
			"type": "user", "isSynthetic": true, "uuid": newRecordUUID(),
			"message": map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": "Stop hook feedback:\n" + text}}},
		})
		if err == nil {
			writeStreamLine(cfg, stampFrame(cfg, line))
		}
	}
}
