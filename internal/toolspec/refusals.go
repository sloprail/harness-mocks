package toolspec

import (
	"errors"
	"sync"
)

// Refusals holds the first error a scenario script of a run ended with, above
// all a call it asked for that the mock does not implement. The sessions of a
// run's sub-agents share their parent's, so an error in a sub-agent's script
// fails the run, not only the sub-agent.
type Refusals struct {
	mu    sync.Mutex
	first error
}

// Set keeps err, when it is a refusal of a call (an *Error), unless the run
// already holds an earlier one; any other error of a script is not kept.
func (r *Refusals) Set(err error) {
	var refusal *Error
	if !errors.As(err, &refusal) {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.first == nil {
		r.first = err
	}
}

// Err is the first error kept, if any.
func (r *Refusals) Err() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.first
}
