package replay

import "runtime"

// Gate limits how many replays a test runs at once (adr/replay-concurrency): a
// recording's hooks run under wall-clock limits, which a machine loaded by many
// replays at once makes the mock miss.
type Gate chan struct{}

// NewGate is a gate as wide as the machine allows: half the CPUs, between one and four.
func NewGate() Gate { return make(Gate, min(max(runtime.NumCPU()/2, 1), 4)) }

// Hold takes a slot, waiting for one, and returns the function that gives it back:
// `defer gate.Hold()()`.
func (g Gate) Hold() func() {
	g <- struct{}{}
	return func() { <-g }
}
