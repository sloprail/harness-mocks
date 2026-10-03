package turnloop

// Continues reports whether the end-of-turn hooks ask the turn to go on: one of
// them blocked, by its exit status or by what it printed. The reason it gave is
// handed back to the agent as the prompt that continues the turn, and the
// caller sees one result, at the turn's real end, not one per continuation.
//
// sr:capability stop-block-continuation
func Continues(blockedByStatus, blockedByDecision bool) bool {
	return blockedByStatus || blockedByDecision
}

// Verdict is what one end-of-turn hook decided: to block (with the reason it
// gives), or to halt, which ends the turn.
type Verdict struct {
	Block  bool
	Reason string
	Halt   bool
}

// Resolve is what all the end-of-turn hooks that ran decide together: whether
// the turn goes on (see Continues), and with which reason, the first block's. A
// hook that halts takes precedence over every block, however many there are and
// whichever ran first: the turn ends.
func Resolve(verdicts []Verdict) (reason string, again bool) {
	for _, v := range verdicts {
		if v.Halt {
			return "", false
		}
	}
	for _, v := range verdicts {
		if v.Block {
			return v.Reason, true
		}
	}
	return "", false
}

// AfterBlock reports whether a block still continues the turn, given how many
// blocks in a row there have been, this one included: the turn goes on only a
// limited number of times in a row, and once blockCap blocks have continued it
// the next is overridden and the turn ends. A blockCap of zero is no limit.
//
// sr:capability stop-block-cap
func AfterBlock(blocksInARow, blockCap int) bool {
	return blockCap == 0 || blocksInARow <= blockCap
}
