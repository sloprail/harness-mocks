package subagents

import (
	"context"
	"sync"
	"time"
)

// Wait states of a sub-agent a wait tells of.
const (
	WaitCompleted = "completed"
	WaitRunning   = "running"
	WaitNotFound  = "not_found"
)

// Handle is a started sub-agent as a wait sees it: whether it has ended, and its
// final report.
type Handle struct {
	Finished func() bool
	Report   func() string
}

// Waits is the sub-agents a run has started, by id, and the wait on them.
// A run has one table: an id of another run is not_found only by being absent.
type Waits struct{ started sync.Map }

// Add records a started sub-agent.
func (w *Waits) Add(id string, h Handle) { w.started.Store(id, h) }

// WaitState is where one sub-agent a wait named stands.
type WaitState struct {
	ID     string
	Status string // WaitCompleted, WaitRunning or WaitNotFound
	Report string // a completed one's final report
}

// WaitResult is what a wait tells: each named sub-agent's state, in the order
// named (empty when the wait timed out), and whether it timed out.
type WaitResult struct {
	States   []WaitState
	TimedOut bool
}

// Timeout is a wait's timeout: ms when given, clamped into [lo, hi], else def (all milliseconds).
func Timeout(ms *int, def, lo, hi int) time.Duration {
	if ms == nil {
		return time.Duration(def) * time.Millisecond
	}
	return time.Duration(min(max(*ms, lo), hi)) * time.Millisecond
}

// sr:capability foreground-subagent-result
//
// Wait waits until one of the named sub-agents has finished, or timeout is up,
// and tells where each stands. Names no sub-agent of the run: it returns at once.
func (w *Waits) Wait(ctx context.Context, ids []string, timeout time.Duration) (WaitResult, error) {
	var known []Handle
	for _, id := range ids {
		if v, ok := w.started.Load(id); ok {
			known = append(known, v.(Handle))
		}
	}
	timedOut := len(known) > 0
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for timedOut {
		for _, h := range known {
			if h.Finished() {
				timedOut = false
			}
		}
		if !timedOut {
			break
		}
		select {
		case <-timer.C:
			return WaitResult{TimedOut: true}, nil
		case <-ctx.Done():
			return WaitResult{}, ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
	var states []WaitState
	for _, id := range ids {
		v, ok := w.started.Load(id)
		switch h, _ := v.(Handle); {
		case !ok:
			states = append(states, WaitState{ID: id, Status: WaitNotFound})
		case h.Finished():
			states = append(states, WaitState{ID: id, Status: WaitCompleted, Report: h.Report()})
		default:
			states = append(states, WaitState{ID: id, Status: WaitRunning})
		}
	}
	return WaitResult{States: states}, nil
}
