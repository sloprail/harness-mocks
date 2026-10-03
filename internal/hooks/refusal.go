package hooks

import "strings"

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
// A harness that tells the agent every refusing hook's reason passes the
// separator to put between them (tellAll): the reasons are then all of the
// refusing hooks', in the order given.
func PreToolRefusal(votes []PreToolVote, tellAll ...string) (refused bool, reason string) {
	var reasons []string
	for _, v := range votes {
		if r, why := PreToolDecision(v.Blocked, v.BlockReason, v.Denied, v.DenyReason); r {
			if len(tellAll) == 0 {
				return true, why
			}
			reasons = append(reasons, why)
		}
	}
	if len(tellAll) == 0 {
		return false, ""
	}
	return len(reasons) > 0, strings.Join(reasons, tellAll[0])
}
