package subagents

import "sync"

// SpawnLog is the ids of the sub-agents an agent has started, in order: the position
// a script's gate names a sub-agent by.
type SpawnLog struct {
	mu      sync.Mutex
	ids     []string
	settled map[string]bool // sub-agents that ran to their end inside the call that started them
}

// Add records a sub-agent the agent has started. A nil SpawnLog records nothing.
func (l *SpawnLog) Add(id string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.ids = append(l.ids, id)
	l.mu.Unlock()
}

// AddSettled records a sub-agent the agent started that has ended by the time the call that started
// it returns (a foreground one): a gate that waits for it has nothing to wait for. A nil SpawnLog
// records nothing.
func (l *SpawnLog) AddSettled(id string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.ids = append(l.ids, id)
	if l.settled == nil {
		l.settled = map[string]bool{}
	}
	l.settled[id] = true
	l.mu.Unlock()
}

func (l *SpawnLog) isSettled(id string) bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.settled[id]
}

func (l *SpawnLog) at(k int) (string, bool) {
	if l == nil {
		return "", false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if k < 0 || k >= len(l.ids) {
		return "", false
	}
	return l.ids[k], true
}
