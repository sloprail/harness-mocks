package runner

// writeResult streams a turn's result, or holds it while a background agent works (tasks.Results).
func (b *backgroundTasks) writeResult(cfg Config, line []byte) {
	for _, l := range b.results.Offer(line, b.AgentsRunning(cfg.AgentID)) {
		writeStreamLine(cfg, withSubagentStats(l, b)) // the tally as it stands as the frame is written, after a held one's agents ended
	}
}

// endRun ends the run: the results held are written, then what is left in the background is
// reaped (the shells are killed after a grace and their frames follow the result, recorded: runs/bgbash).
func (b *backgroundTasks) endRun(cfg Config) {
	for _, l := range b.results.Drain() {
		writeStreamLine(cfg, withSubagentStats(l, b))
	}
	b.ReapAtExit(cfg.AgentID, printReapGrace)
}
