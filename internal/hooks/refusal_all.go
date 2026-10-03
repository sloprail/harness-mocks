package hooks

import "strings"

// PreToolRefusalAll is PreToolRefusal for a harness that tells the agent the
// reason of every hook that refused the call, not the first's alone: each
// refusing hook's reason (a block's or a deny's, by PreToolDecision), in the
// order given, one after another with sep between them. One refusal still
// refuses the call, whatever the others decided.
func PreToolRefusalAll(votes []PreToolVote, sep string) (refused bool, reason string) {
	var reasons []string
	for _, v := range votes {
		if r, why := PreToolDecision(v.Blocked, v.BlockReason, v.Denied, v.DenyReason); r {
			reasons = append(reasons, why)
		}
	}
	return len(reasons) > 0, strings.Join(reasons, sep)
}
