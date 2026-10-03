package subagents

// Part of the foreground-subagent-result capability (its core marker is on HandBack).
//
// TurnLimit is a sub-agent's cap on its turns: the turns in which it called a
// tool are counted, and the run ends at the cap, with no report and no stop
// hook, its parent told so by a note.
type TurnLimit struct {
	// Max is the turns it may take.
	Max   int
	turns int
	hit   bool
}

// Step counts a turn that called a tool and reports whether the run is now at
// its limit. A nil limit, or one of no turns, is never at it.
func (l *TurnLimit) Step() bool {
	if l == nil || l.Max <= 0 {
		return false
	}
	l.turns++
	l.hit = l.turns >= l.Max
	return l.hit
}

// Reached reports whether the sub-agent stopped at its limit.
func (l *TurnLimit) Reached() bool { return l != nil && l.hit }

// Limited is the hooks of a sub-agent that may stop at a limit: a run that
// stopped at it fires no stop hook, so one cannot block it.
func Limited(h Hooks, l *TurnLimit) Hooks {
	stop := h.Stop
	h.Stop = func(active bool, last string) (bool, string) {
		if l.Reached() {
			return false, ""
		}
		return stop(active, last)
	}
	return h
}
