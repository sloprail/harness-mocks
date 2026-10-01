package hooks

// PreToolDecision is what a before-tool hook decides about a tool call: it is
// refused when the hook blocked it by its exit status, or accepted it but
// denied the call in its output. A refused call does not run; the reason
// reaches the agent as the call's error result, and the turn goes on. A block
// by status outranks the output: its reason is the block's.
//
// sr:capability pretooluse-refusal
func PreToolDecision(blocked bool, blockReason string, denied bool, denyReason string) (refused bool, reason string) {
	switch {
	case blocked:
		return true, blockReason
	case denied:
		return true, denyReason
	default:
		return false, ""
	}
}

// permissionRank orders before-tool permission decisions: when several hooks
// decide one call, the highest wins (deny > defer > ask > allow).
var permissionRank = map[string]int{"allow": 1, "ask": 2, "defer": 3, "deny": 4}

// StrongerPermission is whichever of two hooks' permission decisions wins.
func StrongerPermission(a, b string) string {
	if permissionRank[b] > permissionRank[a] {
		return b
	}
	return a
}

// StrongerDecision is whichever of two hooks' top-level decisions wins: a
// "block" from any hook stands, and another hook's "approve" does not undo
// it; otherwise the later decision is kept.
func StrongerDecision(a, b string) string {
	if a == "block" || b == "" {
		return a
	}
	return b
}
