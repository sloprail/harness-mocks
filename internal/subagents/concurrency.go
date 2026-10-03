package subagents

import "strconv"

// DefaultConcurrentLimit is how many sub-agents may run in a session at once
// unless the harness is told otherwise.
const DefaultConcurrentLimit = 20

// ConcurrentLimit is the limit a setting gives: a positive whole number in
// plain digits sets it, anything else leaves the default, so the setting can
// adjust the limit but not lift it.
func ConcurrentLimit(setting string) int {
	for _, r := range setting {
		if r < '0' || r > '9' {
			return DefaultConcurrentLimit
		}
	}
	n, err := strconv.Atoi(setting)
	if err != nil || n <= 0 {
		return DefaultConcurrentLimit
	}
	return n
}

// AtConcurrentLimit reports whether another sub-agent may not be spawned: the
// running ones already are the limit.
func AtConcurrentLimit(running, limit int) bool { return running >= limit }
