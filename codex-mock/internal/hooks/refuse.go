package hooks

import corehooks "github.com/sloprail/harness-mocks/internal/hooks"

// Refusal is whether the before-tool hooks of one call refuse it, and the
// reason the agent is told: core's rule over what each hook decided.
//
// sr:provides pretooluse-refusal/codex
// sr:docs https://developers.openai.com/codex/hooks#pretooluse
func Refusal(ds []Decision) (refused bool, reason string) {
	votes := make([]corehooks.PreToolVote, len(ds))
	for i, d := range ds {
		votes[i] = corehooks.PreToolVote{Blocked: d.Blocked, BlockReason: d.BlockReason, Denied: d.Denied, DenyReason: d.DenyReason}
	}
	return corehooks.PreToolRefusal(votes)
}
