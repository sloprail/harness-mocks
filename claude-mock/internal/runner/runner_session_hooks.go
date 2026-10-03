package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
)

// fireSessionEnd fires SessionEnd the way a `claude -p` session ends: reason
// "other" (claude 2.1.282). Its output is not recorded — a SessionEnd hook
// that printed left nothing in the real transcript. A handler that fails is
// shown to the user only, on the harness's stderr (recorded:
// snapshots/runs/hook-exit-codes).
// sr:docs https://code.claude.com/docs/en/hooks#sessionend
func fireSessionEnd(ctx context.Context, cfg Config, inv *hooks.Invoker) {
	_, runs, _ := inv.FireRuns(ctx, hooks.Input{
		SessionID:     cfg.SessionID,
		Cwd:           cfg.Cwd,
		HookEventName: hooks.EventSessionEnd,
		Reason:        endReason(corehooks.SessionEndReason(false, corehooks.EndOther)),
	})
	for _, r := range runs {
		if r.ExitCode != 0 && cfg.Stderr != nil {
			fmt.Fprintf(cfg.Stderr, "SessionEnd hook [%s] failed: %s\n", r.Command, strings.TrimSpace(r.Stderr))
		}
	}
}

// fireSessionStart fires the SessionStart hook with the given source
// ("startup" | "resume" | "fork" | "compact"), surfaces any additionalContext
// the hooks returned (emitting a system record on the output stream, as real
// Claude Code injects a SessionStart hook's additionalContext into the
// session context — most notably on source="compact", to re-seed a compacted
// window), and returns it. An exit 2 does not stop anything: SessionStart
// cannot block (docs), and the handler's attachment records it as a
// non-blocking error.
// sr:docs https://code.claude.com/docs/en/hooks#sessionstart
func fireSessionStart(ctx context.Context, cfg Config, inv *hooks.Invoker, kind corehooks.StartKind) (string, error) {
	in := hooks.Input{
		SessionID:     cfg.SessionID,
		Cwd:           cfg.Cwd,
		HookEventName: hooks.EventSessionStart,
		Source:        startSource(kind),
	}
	if kind == corehooks.StartResumed || kind == corehooks.StartForked {
		in.ResumeFields = resumeFields(cfg.sessionFile)
	}
	if kind == corehooks.StartResumed {
		in.SessionTitle = sessionTitleOf(cfg.sessionFile)
	}
	ssOut, runs, ferr := inv.FireRuns(ctx, in)
	// Only the SessionStart of a session resumed from another directory is told
	// the path under that directory's project folder; every hook after it is told
	// the file the session really writes (recorded: snapshots/runs/forkresume, step 4).
	if cfg.sessionFile != "" {
		inv.SetTranscriptPath(cfg.sessionFile)
	}
	writeSessionStartFrames(cfg, in, runs)
	if in.SessionTitle != "" { // after the hooks' frames, before init (recorded: snapshots/runs/resume-name)
		writeFrame(cfg, map[string]any{"type": "system", "subtype": "session_title_changed", "title": in.SessionTitle})
	}
	var blockErr *hooks.BlockError
	if errors.As(ferr, &blockErr) && corehooks.BlocksSessionStart(corehooks.Blocked) {
		return "", ferr
	}
	ac := promptContextFrom(ssOut)
	if ac != "" {
		emitSystemContext(cfg, "session_start", ac)
	}
	return ac, nil
}

// additionalContextFrom extracts the additionalContext a hook returned, if any.
// Real Claude Code appends this to the model's context; the mock forwards it to
// the script via A10N_MOCK_ADDITIONAL_CONTEXT.
// promptContextFrom is the context a UserPromptSubmit or SessionStart hook
// adds: its JSON additionalContext, else its plain-text stdout.
// sr:docs https://code.claude.com/docs/en/hooks#exit-code-0
//
// sr:provides hook-additional-context/claude
func promptContextFrom(out hooks.Output) string {
	return corehooks.ContextOf(additionalContextFrom(out), out.PlainText, true)
}

func additionalContextFrom(out hooks.Output) string {
	if out.HookSpecificOutput != nil {
		return out.HookSpecificOutput.AdditionalContext
	}
	return ""
}

// emitSystemContext writes a JSONL system record carrying additionalContext to the
// output stream, mirroring how real Claude Code surfaces a hook's injected context
// (e.g. a SessionStart compact re-seed). source identifies the originating hook.
func emitSystemContext(cfg Config, source, additionalContext string) {
	rec := map[string]any{
		"type":              "system",
		"subtype":           "hook_additional_context",
		"source":            source,
		"additionalContext": additionalContext,
	}
	if b, err := json.Marshal(rec); err == nil {
		writeStreamLine(cfg, b)
	}
}

// promptBlocked is what real Claude Code leaves behind when a UserPromptSubmit
// hook blocks the prompt (exit 2): the prompt never reaches the model; a
// warning names the hook and the original prompt, on the transcript and the
// stream, and the run ends with a result carrying the same text, successfully
// (recorded: snapshots/runs/prompt-blocked). Any other error is returned.
// sr:docs https://code.claude.com/docs/en/hooks#what-a-blocked-prompt-leaves-behind
func promptBlocked(cfg Config, tr *transcript, err error) error {
	var be *hooks.BlockError
	if !errors.As(err, &be) {
		return fmt.Errorf("claude-mock: UserPromptSubmit hook blocked: %w", err)
	}
	text := "UserPromptSubmit operation blocked by hook:\n" + be.Quoted()
	if !be.SuppressPrompt {
		text += "\n\nOriginal prompt: " + cfg.Prompt
	}
	if tr != nil {
		tr.persistMap(map[string]any{
			"type": "system", "subtype": "informational", "content": text,
			"isMeta": false, "level": "warning", "preventContinuation": true,
		})
	}
	writeFrame(cfg, map[string]any{
		"type": "system", "subtype": "informational", "content": text,
		"level": "warning", "prevent_continuation": true,
	})
	if line, merr := json.Marshal(map[string]any{
		"type": "result", "subtype": "success", "is_error": false, "num_turns": 0,
		"result": text, "session_id": cfg.SessionID,
	}); merr == nil {
		writeStreamLine(cfg, line)
	}
	return nil
}
