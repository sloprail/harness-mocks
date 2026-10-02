package hooks

import "strings"

// FailsClosed reports whether a command that gave no result to read blocks the
// action all the same: it timed out, and the harness has a setting (strict:
// the same one VerdictOf takes) that makes a hook's failures block. Without it
// the action goes on.
func FailsClosed(o Outcome, strict bool) bool { return strict && !o.Counts() && o.TimedOut }

// SilentFails reports whether a command that exited cleanly but printed nothing
// blocks the action: only under the same strict setting, for an event whose
// decision is read from the output.
func SilentFails(stdout string, strict bool) bool { return strict && strings.TrimSpace(stdout) == "" }
