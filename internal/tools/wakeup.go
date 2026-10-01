package tools

import (
	"math"
	"sync"
	"time"
)

// Bounds of a wake-up delay: a requested delay is clamped into them.
const (
	MinWakeupSeconds = 60
	MaxWakeupSeconds = 3600
)

// WakeupRequest is a call asking to be woken later.
type WakeupRequest struct {
	// DelaySeconds is how long to wait, Prompt what to resume with, Reason why;
	// Noop is required, with Stop false, and Stop cancels the pending wake-up.
	DelaySeconds *int
	Prompt       *string
	Reason       string
	Noop         *bool
	Stop         bool
}

// MissingArgError is a request without an argument it needs.
type MissingArgError struct{ Arg string }

func (e *MissingArgError) Error() string { return "missing required argument " + e.Arg }

// Wakeup is a pending wake-up.
type Wakeup struct {
	ID     string
	At     time.Time
	Prompt string
}

// WakeupResult is what a request amounts to.
type WakeupResult struct {
	// At is when the wake-up will fire and DelaySeconds the delay it was clamped
	// to; Clamped says the request was outside the bounds.
	At           time.Time
	DelaySeconds int
	Clamped      bool
	// Stopped and Cancelled report a Stop request and how many pending wake-ups
	// it cancelled.
	Stopped   bool
	Cancelled int
}

// Wakeups is a session's pending wake-up: at most one, a new request replacing
// the last.
type Wakeups struct {
	mu      sync.Mutex
	pending *Wakeup
}

// NewWakeups is a session with nothing pending.
func NewWakeups() *Wakeups { return &Wakeups{} }

// Schedule acts on a request at time now. A stop request cancels the pending
// wake-up. Any other needs a delay, a prompt and the noop flag, else it is a
// *MissingArgError; its delay is clamped to the bounds, and the wake-up is set
// for that long from now, rounded up to a whole minute, replacing the pending
// one. A valid request is acknowledged and nothing fires in the mock: the turn
// goes on as the wake-up having fired.
//
// sr:capability schedule-wakeup
func (w *Wakeups) Schedule(req WakeupRequest, now time.Time, newID func() string) (WakeupResult, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if req.Stop {
		cancelled := 0
		if w.pending != nil {
			cancelled, w.pending = 1, nil
		}
		return WakeupResult{Stopped: true, Cancelled: cancelled}, nil
	}
	switch {
	case req.DelaySeconds == nil:
		return WakeupResult{}, &MissingArgError{Arg: "delaySeconds"}
	case req.Prompt == nil || *req.Prompt == "":
		return WakeupResult{}, &MissingArgError{Arg: "prompt"}
	case req.Noop == nil:
		return WakeupResult{}, &MissingArgError{Arg: "noop"}
	}
	delay := min(max(*req.DelaySeconds, MinWakeupSeconds), MaxWakeupSeconds)
	at := now.Add(time.Duration(delay) * time.Second).Truncate(time.Minute)
	if at.Before(now.Add(time.Duration(delay) * time.Second)) {
		at = at.Add(time.Minute)
	}
	w.pending = &Wakeup{ID: newID(), At: at, Prompt: *req.Prompt}
	return WakeupResult{At: at, DelaySeconds: delay, Clamped: delay != *req.DelaySeconds}, nil
}

// Pending is the wake-up that has not fired yet, if any.
func (w *Wakeups) Pending() []Wakeup {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.pending == nil {
		return nil
	}
	return []Wakeup{*w.pending}
}

// SecondsUntil is how long from now a result's wake-up is, rounded to whole
// seconds.
func (r WakeupResult) SecondsUntil(now time.Time) int {
	return int(math.Round(r.At.Sub(now).Seconds()))
}
