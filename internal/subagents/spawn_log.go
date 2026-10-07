package subagents

import (
	"context"
	"sync"
)

// SpawnLog is the ids of the sub-agents an agent has started, in order: the position
// a script's gate names a sub-agent by.
type SpawnLog struct {
	mu       sync.Mutex
	ids      []string
	settled  map[string]bool      // sub-agents that ran to their end inside the call that started them
	progress map[string]*Progress // how far each sub-agent has got, once it has begun
	spawns   map[string]*SpawnLog // the sub-agents each sub-agent has started in turn, once it has begun
	begun    chan struct{}        // closed and replaced when a sub-agent is added, or its progress or spawns are set
}

// signal wakes whoever waits on the log to change. The caller holds l.mu.
func (l *SpawnLog) signal() {
	if l.begun != nil {
		close(l.begun)
	}
	l.begun = make(chan struct{})
}

// SetSpawns records the sub-agents a sub-agent the agent started has started in turn.
func (l *SpawnLog) SetSpawns(id string, sub *SpawnLog) {
	if l == nil {
		return
	}
	l.mu.Lock()
	if l.spawns == nil {
		l.spawns = map[string]*SpawnLog{}
	}
	l.spawns[id] = sub
	l.signal()
	l.mu.Unlock()
}

// awaitSub is the sub-agent at position k among those started, waiting until it has been (empty
// if ctx ends first); awaitSpawns is the log of what that sub-agent started, once it has begun.
func (l *SpawnLog) awaitSub(ctx context.Context, k int) (string, bool) {
	for {
		l.mu.Lock()
		if l.begun == nil {
			l.begun = make(chan struct{})
		}
		wait := l.begun
		if k >= 0 && k < len(l.ids) {
			id := l.ids[k]
			l.mu.Unlock()
			return id, true
		}
		l.mu.Unlock()
		select {
		case <-wait:
		case <-ctx.Done():
			return "", false
		}
	}
}

func (l *SpawnLog) awaitSpawns(ctx context.Context, id string) *SpawnLog {
	for {
		l.mu.Lock()
		if l.begun == nil {
			l.begun = make(chan struct{})
		}
		wait, sub := l.begun, l.spawns[id]
		l.mu.Unlock()
		if sub != nil {
			return sub
		}
		select {
		case <-wait:
		case <-ctx.Done():
			return nil
		}
	}
}

// Add records a sub-agent the agent has started. A nil SpawnLog records nothing.
func (l *SpawnLog) Add(id string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.ids = append(l.ids, id)
	l.signal()
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
	l.signal()
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

// ID is the sub-agent at position k among those the agent started, if it has started that many.
func (l *SpawnLog) ID(k int) (string, bool) { return l.at(k) }
