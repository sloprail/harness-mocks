package hooks

import "time"

// Counts reports whether a command's result is read at all: a command that
// timed out was cancelled, its output discarded, so it renders no decision,
// and a command that could not start has none to read either.
func (o Outcome) Counts() bool { return o.Started && !o.TimedOut }

// ActedBlock is which of the commands of one event a block is acted on for.
// Every matching command runs to completion before the results are merged,
// whatever one of them decides; of those that block (by exit status, as the
// event reads it: strict is VerdictOf's), the one that finished last is acted
// on. ok is false when none blocked.
//
// sr:capability hooks-all-matching-run
func ActedBlock(outs []Outcome, strict bool) (index int, ok bool) {
	last := 0
	for i, o := range outs {
		if o.Counts() && VerdictOf(o.Exit, strict) == Blocked && o.Done >= last {
			index, ok, last = i, true, o.Done
		}
	}
	return index, ok
}

// DefaultTimeout is how long a command may run: its own timeout when it has
// one, else the harness's default for the event.
//
// sr:capability hook-timeout
func DefaultTimeout(own, harness time.Duration) time.Duration {
	if own > 0 {
		return own
	}
	return harness
}
