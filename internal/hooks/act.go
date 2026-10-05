package hooks

import "time"

// Counts reports whether a command's result is read at all: a command that
// timed out was cancelled, its output discarded, so it renders no decision,
// and a command that could not start has none to read either.
func (o Outcome) Counts() bool { return o.Started && !o.TimedOut }

// together is how close two commands' finish times must be to count as
// finishing together: commands started at once that end within this of each
// other have no order of their own, and of them the one configured last is the
// one acted on. It is grounded in what claude's real harness did with two
// blockers on one event (runs all-hooks, all-hooks-close-first/-second,
// all-hooks-slow-first): both ending at once (all-hooks): the last configured
// acted, 3 of 3; ending 75 ms apart with the last configured last
// (close-second): it, 3 of 3; ending 1 s apart (slow-first): the one that
// finished last, whichever it was configured as. Ending 75 ms apart with the
// first configured last (all-hooks-close-first, three samples: A acted, then B,
// then A) the harness itself raced, so no rule matches all of it; a gap that
// small is not told apart reliably. That recording is not kept: it can have no
// replay that is green in every sample.
const together = 50 * time.Millisecond

// ActedBlock is which of the commands of one event a block is acted on for.
// Every matching command runs to completion before the results are merged,
// whatever one of them decides; of those that block (by exit status, as the
// event reads it: strict is VerdictOf's), the one that finished last is acted
// on, and of several that finished together (within `together` of the last)
// the one configured last. ok is false when none blocked.
//
// sr:capability hooks-all-matching-run
func ActedBlock(outs []Outcome, strict bool) (index int, ok bool) {
	var last time.Duration
	for _, o := range outs {
		if o.Counts() && VerdictOf(o.Exit, strict) == Blocked && o.Finished > last {
			last = o.Finished
		}
	}
	for i, o := range outs {
		if o.Counts() && VerdictOf(o.Exit, strict) == Blocked && o.Finished >= last-together {
			index, ok = i, true
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

// Unseen is the hooks of add that have not been configured already: a hook
// listed in more than one place, under the same matcher, runs once. It keeps
// the order of add and drops a repeat within add too.
func Unseen[T comparable](have, add []T) []T {
	seen := map[T]bool{}
	for _, h := range have {
		seen[h] = true
	}
	var out []T
	for _, h := range add {
		if !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	return out
}

// CapTimeout is a timeout limited to max (zero is no limit, and a zero timeout
// stays zero: the default applies).
func CapTimeout(timeout, max time.Duration) time.Duration {
	if max > 0 && timeout > max {
		return max
	}
	return timeout
}
