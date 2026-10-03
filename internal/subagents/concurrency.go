package subagents

// DefaultConcurrentLimit is how many sub-agents may run in a session at once.
const DefaultConcurrentLimit = 20

// AtConcurrentLimit reports whether another sub-agent may not be spawned: the
// running ones already are the limit.
func AtConcurrentLimit(running, limit int) bool { return running >= limit }
