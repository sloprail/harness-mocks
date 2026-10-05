package hooks

import "time"

// Counts reports whether a command's result is read at all: a command that
// timed out was cancelled, its output discarded, so it renders no decision,
// and a command that could not start has none to read either.
func (o Outcome) Counts() bool { return o.Started && !o.TimedOut }

// simultaneous is how close two commands' run times must be to count as
// finishing together: commands started at once that take about as long have no
// order of their own, and the one configured last is the one acted on (recorded:
// claude snapshots/runs/all-hooks, two blockers that both exit at once: the
// second configured is acted on in every sample).
const simultaneous = 150 * time.Millisecond

// ActedBlock is which of the commands of one event a block is acted on for.
// Every matching command runs to completion before the results are merged,
// whatever one of them decides; of those that block (by exit status, as the
// event reads it: strict is VerdictOf's), the one that finished last is acted
// on, and of several that finished together the one configured last. ok is
// false when none blocked.
//
// sr:capability hooks-all-matching-run
func ActedBlock(outs []Outcome, strict bool) (index int, ok bool) {
	var longest time.Duration
	for _, o := range outs {
		if o.Counts() && VerdictOf(o.Exit, strict) == Blocked && o.Took > longest {
			longest = o.Took
		}
	}
	for i, o := range outs {
		if o.Counts() && VerdictOf(o.Exit, strict) == Blocked && o.Took >= longest-simultaneous {
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
