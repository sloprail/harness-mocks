package runner

import (
	"context"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
)

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
func submitPrompt(ctx context.Context, cfg Config, inv *hooks.Invoker, tr *transcript, src corehooks.PromptSource) (extra string, refused bool, err error) {
	if cfg.Prompt == "" || !corehooks.PromptHookFires(src) {
		return "", false, nil
	}
	out, ferr := inv.Fire(ctx, hooks.Input{
		SessionID:     cfg.SessionID,
		Cwd:           cfg.Cwd,
		HookEventName: hooks.EventUserPromptSubmit,
		Prompt:        cfg.Prompt,
	})
	refused, extra = corehooks.PromptOutcome(ferr != nil, promptContextFrom(out))
	if refused {
		return "", true, promptBlocked(cfg, tr, ferr)
	}
	return extra, false, nil
}
