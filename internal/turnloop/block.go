package turnloop

// Continues reports whether the end-of-turn hooks ask the turn to go on: one of
// them blocked, by its exit status or by what it printed. The reason it gave is
// handed back to the agent as the prompt that continues the turn, and the
// caller sees one result, at the turn's real end, not one per continuation.
func Continues(blockedByStatus, blockedByDecision bool) bool {
	return blockedByStatus || blockedByDecision
}

// AfterBlock reports whether a block still continues the turn, given how many
// blocks in a row there have been, this one included: the turn goes on only a
// limited number of times in a row, and once blockCap blocks have continued it
// the next is overridden and the turn ends. A blockCap of zero is no limit.
func AfterBlock(blocksInARow, blockCap int) bool {
	return blockCap == 0 || blocksInARow <= blockCap
}
