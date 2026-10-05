package replay

import "runtime"

// replays limits how many replays run at once in a process (adr/replay-concurrency):
// a recording's hooks run under wall-clock limits, which a machine loaded by many
// replays at once makes the mock miss. Half the CPUs, between one and four.
var replays = make(chan struct{}, min(max(runtime.NumCPU()/2, 1), 4))

// hold takes a slot, waiting for one, and returns the function that gives it back.
func hold() func() {
	replays <- struct{}{}
	return func() { <-replays }
}
