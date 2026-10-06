package tasks

// Results are the result frames of a run's turns that ended while a background agent was still
// working: they are not streamed then, but with the results of the turns the agent's notification
// starts, once no background agent is working (recorded: bgagent, bgagent-nested-launcher,
// bgagent-concurrent-limit show both results last; bgagent-definition, whose agent finished before
// its turn ended, streams each result as its turn ends).
type Results struct {
	held [][]byte
}

// Offer takes the result of a turn that has ended and returns the frames to write now: none while a
// background agent works, else the results held, in order, then this one.
func (r *Results) Offer(line []byte, agentsRunning bool) [][]byte {
	if agentsRunning {
		r.held = append(r.held, line)
		return nil
	}
	return append(r.Drain(), line)
}

// Drain returns the results held, in order, and holds none: the run is ending.
func (r *Results) Drain() [][]byte {
	out := r.held
	r.held = nil
	return out
}
