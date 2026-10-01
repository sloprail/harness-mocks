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

// VerdictOf is the verdict of a hook command's exit status.
//
// sr:capability hook-exit-code-semantics
func VerdictOf(exitCode int) Verdict {
	switch exitCode {
	case 0:
		return Accepted
	case 2:
		return Blocked
	default:
		return NonBlockingError
	}
}
