package hooks

import corehooks "github.com/sloprail/harness-mocks/internal/hooks"

// Refusal is whether the before-tool hooks of one call refuse it, and the
// reason the agent is told. Each hook decides by core's rule; one refusal
// stands whatever the other hooks decided (a deny wins over an allow), and the
// reason is the first refusing hook's, in configuration order.
//
// sr:provides pretooluse-refusal/codex
// sr:docs https://developers.openai.com/codex/hooks#pretooluse
func Refusal(ds []Decision) (refused bool, reason string) {
	for _, d := range ds {
		if r, why := corehooks.PreToolDecision(d.Blocked, d.BlockReason, d.Denied, d.DenyReason); r {
			return true, why
		}
	}
	return false, ""
}
