package runner

import (
	"context"
	"encoding/json"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
)

// fireSessionEnd fires SessionEnd the way a `claude -p` session ends: reason
// "other" (claude 2.1.282). Its output is not recorded — a SessionEnd hook
// that printed left nothing in the real transcript.
// sr:docs https://code.claude.com/docs/en/hooks#sessionend
func fireSessionEnd(ctx context.Context, cfg Config, inv *hooks.Invoker) {
	_, _ = inv.Fire(ctx, hooks.Input{
		SessionID:     cfg.SessionID,
		Cwd:           cfg.Cwd,
		HookEventName: hooks.EventSessionEnd,
		Reason:        "other",
	})
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
func fireSessionStart(ctx context.Context, cfg Config, inv *hooks.Invoker, source string) string {
	ssOut, _ := inv.Fire(ctx, hooks.Input{
		SessionID:     cfg.SessionID,
		Cwd:           cfg.Cwd,
		HookEventName: hooks.EventSessionStart,
		Source:        source,
	})
	ac := additionalContextFrom(ssOut)
	if ac != "" {
		emitSystemContext(cfg, "session_start", ac)
	}
	return ac
}

// additionalContextFrom extracts the additionalContext a hook returned, if any.
// Real Claude Code appends this to the model's context; the mock forwards it to
// the script via A10N_MOCK_ADDITIONAL_CONTEXT.
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
