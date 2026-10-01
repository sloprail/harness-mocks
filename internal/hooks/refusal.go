package hooks

// PreToolVote is what one before-tool hook decided about a call, by its exit
// status (Blocked) and by what it printed (Denied), each with its reason.
type PreToolVote struct {
	Blocked     bool
	BlockReason string
	Denied      bool
	DenyReason  string
}

// PreToolRefusal is whether a call is refused when several hooks vote on it,
// and the reason the agent is told: each hook decides by PreToolDecision, one
// refusal stands whatever the others decided (a deny wins over any other
// decision), and the reason is the first refusing hook's, in the order given.
func PreToolRefusal(votes []PreToolVote) (refused bool, reason string) {
	for _, v := range votes {
		if r, why := PreToolDecision(v.Blocked, v.BlockReason, v.Denied, v.DenyReason); r {
			return true, why
		}
	}
	return false, ""
}
