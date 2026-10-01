// Package hooks is the harness-neutral core of hook handling.
package hooks

// Verdict is what a hook command's exit status decides on its own.
type Verdict int

const (
	// Accepted: exit 0, the command's output is read.
	Accepted Verdict = iota
	// Blocked: exit 2, a blocking error; what it blocks depends on the event.
	Blocked
	// NonBlockingError: any other status; the agent carries on.
	NonBlockingError
)

// VerdictOf is the verdict of a hook command's exit status. A strict event
// (one that fails on any non-zero exit: a harness says which) takes every
// non-zero status as Blocked.
//
// sr:capability hook-exit-code-semantics
func VerdictOf(exitCode int, strict bool) Verdict {
	switch {
	case exitCode == 0:
		return Accepted
	case exitCode == 2, strict:
		return Blocked
	default:
		return NonBlockingError
	}
}
