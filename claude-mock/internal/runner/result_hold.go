package runner

// A turn that ends while a background agent is still working does not stream its result then:
// the real stream holds it, and writes it with the results of the turns the agent's
// notification starts, once no background agent is working (recorded: snapshots/runs/bgagent,
// bgagent-nested-launcher, bgagent-concurrent-limit show both results last; bgagent-definition,
// whose agent finished before the turn ended, streams each result as its turn ends).
// sr:provides print-waits-for-background-agents/claude
func (b *backgroundTasks) writeResult(cfg Config, line []byte) {
	if b.AgentsRunning(cfg.AgentID) {
		b.held = append(b.held, line)
		return
	}
	b.flushResults(cfg)
	writeStreamLine(cfg, line)
}

// flushResults writes the results held, in order.
func (b *backgroundTasks) flushResults(cfg Config) {
	for _, line := range b.held {
		writeStreamLine(cfg, line)
	}
	b.held = nil
}

// endRun ends the run: the held results are written, then what is left in the background is
// reaped (the shells are killed after a grace and their frames follow the result, recorded: runs/bgbash).
func (b *backgroundTasks) endRun(cfg Config) {
	b.flushResults(cfg)
	b.ReapAtExit(cfg.AgentID, printReapGrace)
}
