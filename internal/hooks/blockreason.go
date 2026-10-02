package hooks

// BlockReason reads what an end-of-turn hook (Stop, SubagentStop) said about
// letting the turn end: it blocked when it failed in the way a block is
// signalled (err: an exit status the event reads as a block, whose text is the
// reason), or when what it printed was a decision of "block" (with the reason
// it gave). blocked is false when it let the turn end. It is the reading
// turnloop.Continues acts on.
func BlockReason(err error, decision, reason string) (blocked bool, why string) {
	if err != nil {
		return true, err.Error()
	}
	if decision == "block" {
		return true, reason
	}
	return false, ""
}
