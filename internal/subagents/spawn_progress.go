package subagents

import (
	"context"
)

// SetProgress records how far a sub-agent the agent started has got, once it has begun to run.
func (l *SpawnLog) SetProgress(id string, p *Progress) {
	if l == nil {
		return
	}
	l.mu.Lock()
	if l.progress == nil {
		l.progress = map[string]*Progress{}
	}
	l.progress[id] = p
	l.signal()
	l.mu.Unlock()
}

// awaitProgress is the progress of a sub-agent, waiting until it has begun (nil if ctx ends first).
func (l *SpawnLog) awaitProgress(ctx context.Context, id string) *Progress {
	for {
		l.mu.Lock()
		p := l.progress[id]
		if l.begun == nil {
			l.begun = make(chan struct{})
		}
		wait := l.begun
		l.mu.Unlock()
		if p != nil {
			return p
		}
		select {
		case <-wait:
		case <-ctx.Done():
			return nil
		}
	}
}

// Progress is how far a sub-agent has got, if it has begun (nil if not).
func (l *SpawnLog) Progress(id string) *Progress {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.progress[id]
}
